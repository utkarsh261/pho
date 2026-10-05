package prdetail

import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/utkarsh261/pho/internal/application/cmds"
	"github.com/utkarsh261/pho/internal/diff/anchor"
	"github.com/utkarsh261/pho/internal/diff/difftest"
	"github.com/utkarsh261/pho/internal/diff/parse"
)

// These tests pin where inline comments land: for every diff line in the
// shared fixtures, the path/side/line a draft carries must equal the anchor
// golden in internal/diff/anchor/testdata/golden.

const placementSHA = "0123456789abcdef0123456789abcdef01234567"

type goldenAnchor struct {
	path, side string
	line       int
	ok         bool
}

func readAnchorGolden(t *testing.T, name string) map[[3]int]goldenAnchor {
	t.Helper()
	dir := filepath.Join("..", "..", "..", "diff", "anchor", "testdata", "golden")
	data, err := os.ReadFile(filepath.Join(dir, name+".anchors"))
	if err != nil {
		gz, gzErr := os.ReadFile(filepath.Join(dir, name+".anchors.gz"))
		if gzErr != nil {
			t.Fatalf("read golden %s: %v", name, err)
		}
		zr, err := gzip.NewReader(bytes.NewReader(gz))
		if err != nil {
			t.Fatal(err)
		}
		if data, err = io.ReadAll(zr); err != nil {
			t.Fatal(err)
		}
	}
	out := make(map[[3]int]goldenAnchor)
	for _, row := range strings.Split(string(data), "\n") {
		if row == "" || strings.HasPrefix(row, "file\t") {
			continue
		}
		// fi hi li path kind old new side line
		f := strings.Split(row, "\t")
		if len(f) != 9 {
			t.Fatalf("bad golden row %q", row)
		}
		fi, _ := strconv.Atoi(f[0])
		hi, _ := strconv.Atoi(f[1])
		li, _ := strconv.Atoi(f[2])
		g := goldenAnchor{path: f[3], side: f[7]}
		if g.side != "-" {
			g.line, _ = strconv.Atoi(f[8])
			g.ok = true
		}
		out[[3]int{fi, hi, li}] = g
	}
	return out
}

func loadPlacementModel(t *testing.T, raw string) *PRDetailModel {
	t.Helper()
	dm, err := parse.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	anchor.Generate(dm, placementSHA)
	m := makePRDetail(120, 40, nil, nil)
	m.PRService = &prServiceStub{}
	m.DiffLoading = true
	m, _ = m.Update(cmds.DiffLoaded{Repo: "owner/repo", Number: 1, Diff: *dm})
	m.leftPanel.Focus = FocusContent
	m.activeTab = TabDiff
	return m
}

func TestAnchorForLineMatchesGolden(t *testing.T) {
	t.Parallel()
	for _, fx := range difftest.RawDiffs() {
		golden := readAnchorGolden(t, fx.Name)
		m := loadPlacementModel(t, fx.Raw)
		n := 0
		for fi := range m.Diff.Files {
			f := &m.Diff.Files[fi]
			for hi := range f.Hunks {
				for li := range f.Hunks[hi].Lines {
					want, ok := golden[[3]int{fi, hi, li}]
					if !ok {
						t.Fatalf("%s: line %d/%d/%d missing from golden", fx.Name, fi, hi, li)
					}
					path, line, side, gotOK := anchorForLine(f, &f.Hunks[hi].Lines[li])
					if gotOK != want.ok || (gotOK && (path != want.path || line != want.line || side != want.side)) {
						t.Fatalf("%s: line %d/%d/%d: anchorForLine = %s %s:%d (ok=%v), golden %s %s:%d (ok=%v)",
							fx.Name, fi, hi, li, path, side, line, gotOK, want.path, want.side, want.line, want.ok)
					}
					n++
				}
			}
		}
		if n != len(golden) {
			t.Fatalf("%s: checked %d lines, golden has %d", fx.Name, n, len(golden))
		}
	}
}

// TestDraftPlacementMatchesGolden drives the real visual-mode flow
// (Space, optional j, c, submit) and checks the draft that would be posted.
func TestDraftPlacementMatchesGolden(t *testing.T) {
	t.Parallel()
	for _, fx := range difftest.RawDiffs() {
		golden := readAnchorGolden(t, fx.Name)
		m := loadPlacementModel(t, fx.Raw)
		lines := m.navigableLines
		stride := 1
		if len(lines) > 2000 {
			stride = 7
		}
		checked := 0
		for i, c := range lines {
			lastInHunk := i+1 == len(lines) || lines[i+1].FileIdx != c.FileIdx || lines[i+1].HunkIdx != c.HunkIdx
			if i%stride != 0 && !lastInHunk && c.LineIdx != 0 {
				continue
			}
			for _, span := range []int{0, 1} {
				if span == 1 && lastInHunk {
					continue
				}
				m.drafts = nil
				m.visual.Active = false
				m.setDiffCursor(c)
				if !m.validDiffCursor() {
					continue
				}
				m = pressKey(m, " ")
				if span == 1 {
					m = pressKey(m, "j")
				}
				m = pressKey(m, "c")
				m, _ = m.Update(submitComposeMsg{body: "x"})
				end := golden[[3]int{c.FileIdx, c.HunkIdx, c.LineIdx + span}]
				if !end.ok {
					if len(m.drafts) != 0 {
						t.Fatalf("%s: %v span %d: got draft on a line without an anchor", fx.Name, c, span)
					}
					continue
				}
				if len(m.drafts) != 1 {
					t.Fatalf("%s: %v span %d: expected 1 draft, got %d", fx.Name, c, span, len(m.drafts))
				}
				d := m.drafts[0]
				if d.Path != end.path || d.Line != end.line || d.Side != end.side {
					t.Fatalf("%s: %v span %d: draft %s %s:%d, want %s %s:%d",
						fx.Name, c, span, d.Path, d.Side, d.Line, end.path, end.side, end.line)
				}
				wantStartLine, wantStartSide := 0, ""
				if span == 1 {
					if start := golden[[3]int{c.FileIdx, c.HunkIdx, c.LineIdx}]; start.ok {
						wantStartLine, wantStartSide = start.line, start.side
					}
				}
				if d.StartLine != wantStartLine || d.StartSide != wantStartSide {
					t.Fatalf("%s: %v span %d: draft start %s:%d, want %s:%d",
						fx.Name, c, span, d.StartSide, d.StartLine, wantStartSide, wantStartLine)
				}
				checked++
			}
		}
		if checked == 0 {
			t.Fatalf("%s: no drafts checked", fx.Name)
		}
		t.Logf("%s: checked %d drafts", fx.Name, checked)
	}
}
