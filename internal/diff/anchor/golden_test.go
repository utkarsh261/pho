package anchor

import (
	"bytes"
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/utkarsh261/pho/internal/diff/difftest"
	"github.com/utkarsh261/pho/internal/diff/model"
	"github.com/utkarsh261/pho/internal/diff/parse"
)

var update = flag.Bool("update", false, "overwrite golden files with current output")

// These goldens pin the path, side and line every diff line resolves to —
// the data that decides where an inline comment lands on GitHub. They must
// not change unless comment placement is meant to change.
func TestAnchorGoldens(t *testing.T) {
	for _, fx := range difftest.RawDiffs() {
		t.Run(fx.Name, func(t *testing.T) {
			dm, err := parse.Parse(fx.Raw)
			if err != nil {
				t.Fatal(err)
			}
			Generate(dm, "0123456789abcdef0123456789abcdef01234567")
			got := Dump(dm)
			compressed := len(got) > 64*1024
			name := fx.Name + ".anchors"
			if compressed {
				name += ".gz"
			}
			path := filepath.Join("testdata", "golden", name)
			if *update {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				data := []byte(got)
				if compressed {
					data = gzipBytes(t, data)
				}
				if err := os.WriteFile(path, data, 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read golden (run with -update to create): %v", err)
			}
			if compressed {
				data = gunzipBytes(t, data)
			}
			if want := string(data); got != want {
				t.Fatalf("anchors differ from %s:\n%s", path, firstDiff(want, got))
			}
		})
	}
}

// Dump renders one row per diff line:
// file hunk line path kind old new side anchorLine, tab-separated.
func Dump(dm *model.DiffModel) string {
	var b strings.Builder
	num := func(p *int) string {
		if p == nil {
			return "-"
		}
		return fmt.Sprint(*p)
	}
	for fi, f := range dm.Files {
		fmt.Fprintf(&b, "file\t%d\t%s\told=%s\tstatus=%s\tbinary=%v\t+%d\t-%d\n",
			fi, f.NewPath, f.OldPath, f.Status, f.IsBinary, f.Additions, f.Deletions)
		for hi, h := range f.Hunks {
			for li, dl := range h.Lines {
				side, line, path := "-", "-", "-"
				if len(dl.Anchors) > 0 {
					a := dl.Anchors[0]
					side, line, path = a.Side, num(a.Line), a.Path
				}
				fmt.Fprintf(&b, "%d\t%d\t%d\t%s\t%s\t%s\t%s\t%s\t%s\n",
					fi, hi, li, path, dl.Kind, num(dl.OldLine), num(dl.NewLine), side, line)
			}
		}
	}
	return b.String()
}

func firstDiff(want, got string) string {
	w, g := strings.Split(want, "\n"), strings.Split(got, "\n")
	for i := 0; i < max(len(w), len(g)); i++ {
		var wl, gl string
		if i < len(w) {
			wl = w[i]
		}
		if i < len(g) {
			gl = g[i]
		}
		if wl != gl {
			return fmt.Sprintf("line %d\nwant: %q\ngot:  %q", i+1, wl, gl)
		}
	}
	return "(no line difference)"
}

func gzipBytes(t *testing.T, b []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if _, err := zw.Write(b); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func gunzipBytes(t *testing.T, b []byte) []byte {
	t.Helper()
	zr, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	return out
}
