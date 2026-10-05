package prdetail

import (
	"testing"

	"github.com/utkarsh261/pho/internal/diff/anchor"
	"github.com/utkarsh261/pho/internal/diff/difftest"
	diffmodel "github.com/utkarsh261/pho/internal/diff/model"
	"github.com/utkarsh261/pho/internal/diff/parse"
)

// legacyDiffIndices is the nested-map index the flat lookup replaced, kept
// to prove both answer every lookup the same way.
func legacyDiffIndices(dm *diffmodel.DiffModel) (map[string]map[int]string, map[string]map[int]map[string][3]int) {
	lineIndex := make(map[string]map[int]string)
	anchorIndex := make(map[string]map[int]map[string][3]int)
	for fi, f := range dm.Files {
		for hi, h := range f.Hunks {
			for li, dl := range h.Lines {
				for _, a := range dl.Anchors {
					if a.Path == "" || a.Line == nil {
						continue
					}
					lineNum := *a.Line
					if lineIndex[a.Path] == nil {
						lineIndex[a.Path] = make(map[int]string)
						anchorIndex[a.Path] = make(map[int]map[string][3]int)
					}
					lineIndex[a.Path][lineNum] = dl.Raw
					if anchorIndex[a.Path][lineNum] == nil {
						anchorIndex[a.Path][lineNum] = make(map[string][3]int)
					}
					anchorIndex[a.Path][lineNum][a.Side] = [3]int{fi, hi, li}
				}
			}
		}
	}
	return lineIndex, anchorIndex
}

func TestDiffIndicesMatchLegacy(t *testing.T) {
	t.Parallel()
	fixtures := append(difftest.RawDiffs(), difftest.LongLineDiff(), difftest.Fixture{Name: "collapse", Raw: collapseFixture()})
	for _, fx := range fixtures {
		dm, err := parse.Parse(fx.Raw)
		if err != nil {
			t.Fatal(err)
		}
		anchor.Generate(dm, placementSHA)
		m := makePRDetail(120, 40, nil, nil)
		m.Diff = dm
		lineIndex, anchorIndex := legacyDiffIndices(dm)

		checked := 0
		for path, lines := range lineIndex {
			// Probe every indexed line plus its neighbours (often absent).
			for line := range lines {
				for _, probe := range []int{line - 1, line, line + 1} {
					if got, want := m.lookupDiffLine(path, probe), lineIndex[path][probe]; got != want {
						t.Fatalf("%s: lookupDiffLine(%s, %d) = %q, want %q", fx.Name, path, probe, got, want)
					}
					for _, side := range []string{"LEFT", "RIGHT", "", "BOTH"} {
						wantRef, wantOK := anchorIndex[path][probe][side]
						fi, hi, li, ok := m.findDiffLineAnchor(path, probe, side)
						if ok != wantOK || (ok && [3]int{fi, hi, li} != wantRef) {
							t.Fatalf("%s: findDiffLineAnchor(%s, %d, %s) = %v %v, want %v %v",
								fx.Name, path, probe, side, [3]int{fi, hi, li}, ok, wantRef, wantOK)
						}
					}
					fi, hi, li, ok := m.findDiffLineAnchorAnySide(path, probe)
					wantRef, wantOK := anchorIndex[path][probe]["RIGHT"]
					if !wantOK {
						wantRef, wantOK = anchorIndex[path][probe]["LEFT"]
					}
					if ok != wantOK || (ok && [3]int{fi, hi, li} != wantRef) {
						t.Fatalf("%s: findDiffLineAnchorAnySide(%s, %d) = %v %v, want %v %v",
							fx.Name, path, probe, [3]int{fi, hi, li}, ok, wantRef, wantOK)
					}
					checked++
				}
			}
		}
		if m.lookupDiffLine("no/such/file.go", 1) != "" {
			t.Fatalf("%s: lookup of an unknown path found something", fx.Name)
		}
		if checked == 0 {
			t.Fatalf("%s: nothing checked", fx.Name)
		}
	}
}

func TestDiffIndicesFollowTheCurrentDiff(t *testing.T) {
	t.Parallel()
	m := makePRDetail(120, 40, nil, nil)
	dm, _ := parse.Parse(difftest.RawDiffs()[1].Raw)
	anchor.Generate(dm, placementSHA)
	m.Diff = dm
	path := dm.Files[5].NewPath
	line := *dm.Files[5].Hunks[0].Lines[0].NewLine
	if m.lookupDiffLine(path, line) == "" {
		t.Fatal("expected a hit on the loaded diff")
	}
	// A refresh clears the diff: lookups find nothing instead of reading
	// positions from the old diff.
	m.Diff = nil
	if m.lookupDiffLine(path, line) != "" {
		t.Fatal("lookup must miss while no diff is loaded")
	}
	if _, _, _, ok := m.findDiffLineAnchorAnySide(path, line); ok {
		t.Fatal("anchor lookup must miss while no diff is loaded")
	}
	small, _ := parse.Parse(difftest.RawDiffs()[0].Raw)
	anchor.Generate(small, placementSHA)
	m.Diff = small
	if m.lookupDiffLine(path, line) != "" {
		t.Fatal("lookup must not use positions from the previous diff")
	}
}
