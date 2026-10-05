package pr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/utkarsh261/pho/internal/diff/anchor"
	"github.com/utkarsh261/pho/internal/diff/difftest"
	"github.com/utkarsh261/pho/internal/diff/model"
	"github.com/utkarsh261/pho/internal/diff/parse"
	"github.com/utkarsh261/pho/internal/github/rest"
	pholog "github.com/utkarsh261/pho/internal/log"
)

func loadFilesFixture(t *testing.T, name string) []rest.ChangedFile {
	t.Helper()
	var files []rest.ChangedFile
	if err := json.Unmarshal(difftest.FilesJSON(name), &files); err != nil {
		t.Fatal(err)
	}
	return files
}

func parseWithAnchors(t *testing.T, raw string) *model.DiffModel {
	t.Helper()
	dm, err := parse.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	anchor.Generate(dm, "0123456789abcdef0123456789abcdef01234567")
	return dm
}

// The same PR fetched as a raw diff and through the per-file API must give
// every line the same numbers and anchors, or comments made on a large PR
// would land on different lines than on a small one.
func TestFilesFallbackMatchesRawDiff(t *testing.T) {
	t.Parallel()
	raw := parseWithAnchors(t, difftest.RawDiffs()[1].Raw) // ts64457
	files := loadFilesFixture(t, "ts64457")
	viaFiles := parseWithAnchors(t, unifiedDiffFromFiles(files))

	if len(viaFiles.Files) != len(raw.Files) {
		t.Fatalf("file count: per-file %d, raw %d", len(viaFiles.Files), len(raw.Files))
	}
	compared, unavailable := 0, 0
	for i := range raw.Files {
		want, got := raw.Files[i], viaFiles.Files[i]
		if got.NewPath != want.NewPath || got.OldPath != want.OldPath || got.Status != want.Status {
			t.Fatalf("file %d: per-file %s→%s (%s), raw %s→%s (%s)",
				i, got.OldPath, got.NewPath, got.Status, want.OldPath, want.NewPath, want.Status)
		}
		if got.Additions != want.Additions || got.Deletions != want.Deletions {
			t.Errorf("%s: per-file +%d -%d, raw +%d -%d", want.NewPath, got.Additions, got.Deletions, want.Additions, want.Deletions)
		}
		if got.PatchUnavailable {
			unavailable++
			if len(got.Hunks) != 0 {
				t.Errorf("%s: patch-unavailable file has hunks", want.NewPath)
			}
			continue
		}
		// GitHub's per-file patch can merge nearby hunks that the raw diff
		// keeps apart, adding the context lines between them. Every raw line
		// must still appear with the same numbers, text and anchor, and any
		// extra line must be context.
		if msg := sameLines(got, want); msg != "" {
			t.Fatalf("%s: %s", want.NewPath, msg)
		}
		compared++
	}
	if unavailable != 3 {
		t.Errorf("expected 3 files without a patch in the fixture, got %d", unavailable)
	}
	t.Logf("compared %d files line by line, %d without a patch", compared, unavailable)
}

func TestFilesFallbackKubernetes(t *testing.T) {
	t.Parallel()
	files := loadFilesFixture(t, "k8s142410")
	dm := parseWithAnchors(t, unifiedDiffFromFiles(files))
	if len(dm.Files) != len(files) {
		t.Fatalf("parsed %d files from %d", len(dm.Files), len(files))
	}
	for i, f := range files {
		got := dm.Files[i]
		if got.NewPath != f.Filename {
			t.Fatalf("file %d: path %q, want %q", i, got.NewPath, f.Filename)
		}
		wantStatus := f.Status
		if wantStatus != "added" && wantStatus != "removed" && wantStatus != "renamed" {
			wantStatus = "modified"
		}
		if got.Status != wantStatus {
			t.Errorf("%s: status %q, want %q", f.Filename, got.Status, wantStatus)
		}
		if f.PreviousFilename != "" && got.OldPath != f.PreviousFilename {
			t.Errorf("%s: old path %q, want %q", f.Filename, got.OldPath, f.PreviousFilename)
		}
		if got.Additions != f.Additions || got.Deletions != f.Deletions {
			t.Errorf("%s: +%d -%d, want +%d -%d", f.Filename, got.Additions, got.Deletions, f.Additions, f.Deletions)
		}
		if got.PatchUnavailable != (f.Patch == "" && f.Changes > 0) {
			t.Errorf("%s: PatchUnavailable = %v", f.Filename, got.PatchUnavailable)
		}
	}
}

type lineKey struct {
	kind     string
	old, new int
}

func keyOf(l model.DiffLine) lineKey {
	k := lineKey{kind: l.Kind, old: -1, new: -1}
	if l.OldLine != nil {
		k.old = *l.OldLine
	}
	if l.NewLine != nil {
		k.new = *l.NewLine
	}
	return k
}

// sameLines returns "" when every line of want appears in got with identical
// content, and every line only in got is context. The raw diff's phantom
// trailing line (from the final newline) is matched like any other.
func sameLines(got, want model.DiffFile) string {
	have := make(map[lineKey]model.DiffLine)
	for _, h := range got.Hunks {
		for _, l := range h.Lines {
			have[keyOf(l)] = l
		}
	}
	seen := make(map[lineKey]bool)
	for _, h := range want.Hunks {
		for _, l := range h.Lines {
			k := keyOf(l)
			g, ok := have[k]
			if !ok {
				return fmt.Sprintf("raw line %+v missing from per-file diff", k)
			}
			// The files API shows control characters in caret notation
			// (ESC as "^["); that changes display text, not placement.
			if caret(g.Raw) == caret(l.Raw) {
				g.Raw = l.Raw
			}
			if !reflect.DeepEqual(g, l) {
				return fmt.Sprintf("line %+v differs: per-file %q, raw %q", k, g.Raw, l.Raw)
			}
			seen[k] = true
		}
	}
	for k := range have {
		if !seen[k] && k.kind != "context" {
			return fmt.Sprintf("per-file diff has extra %s line %+v", k.kind, k)
		}
	}
	return ""
}

func caret(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == 0x7f:
			b.WriteString("^?")
		case c < 0x20 && c != '\t':
			b.WriteByte('^')
			b.WriteByte(c + 64)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// fallbackServer answers the diff endpoints with 406 and serves files from
// the ts64457 fixture, under prefix ("" for github.com, "/api/v3" for GHES).
func fallbackServer(t *testing.T, prefix string, files []rest.ChangedFile) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, prefix+"/repos/owner/repo/") {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		switch {
		case r.Header.Get("Accept") == "application/vnd.github.v3.diff":
			w.WriteHeader(http.StatusNotAcceptable)
			_, _ = w.Write([]byte(`{"message":"Sorry, the diff exceeded the maximum number of files (300).","errors":[{"code":"too_large"}]}`))
		case strings.HasSuffix(r.URL.Path, "/files") || strings.Contains(r.URL.Path, "/commits/"):
			start := (page - 1) * 100
			end := min(start+100, len(files))
			if end < len(files) {
				w.Header().Set("Link", fmt.Sprintf(`<%s?per_page=100&page=%d>; rel="next", <%s?per_page=100&page=%d>; rel="last"`,
					r.URL.Path, page+1, r.URL.Path, (len(files)+99)/100))
			}
			var body any = files[start:end]
			if strings.Contains(r.URL.Path, "/commits/") {
				body = map[string]any{"files": files[start:end]}
			}
			_ = json.NewEncoder(w).Encode(body)
		default:
			t.Errorf("unexpected request %s", r.URL)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestLoadDiffFallsBackToFilesWhenTooLarge(t *testing.T) {
	t.Parallel()
	files := loadFilesFixture(t, "ts64457")
	for _, prefix := range []string{"", "/api/v3"} {
		srv := fallbackServer(t, prefix, files)
		svc := &PRService{
			Cache: newTestCoordinator(t),
			REST:  &rest.Client{BaseURL: srv.URL + prefix, Token: "tok"},
			Owner: "owner", Repo: "repo",
			Log: pholog.NewNop(), Now: func() time.Time { return frozenNow },
		}
		dm, _, err := svc.LoadDiff(context.Background(), testRepo(), 42, "abc", false)
		if err != nil {
			t.Fatalf("prefix %q: %v", prefix, err)
		}
		if len(dm.Files) != len(files) || dm.PartialFiles {
			t.Fatalf("prefix %q: got %d files (partial=%v), want %d", prefix, len(dm.Files), dm.PartialFiles, len(files))
		}
		if len(dm.Files[0].Hunks[0].Lines[0].Anchors) == 0 {
			t.Fatalf("prefix %q: anchors not generated", prefix)
		}

		commit, err := svc.LoadCommitDiff(context.Background(), testRepo(), "abc", false)
		if err != nil {
			t.Fatalf("prefix %q commit: %v", prefix, err)
		}
		if len(commit.Files) != len(files) {
			t.Fatalf("prefix %q commit: got %d files", prefix, len(commit.Files))
		}
	}
}

func TestLoadDiffFallbackFailureKeepsError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotAcceptable)
	}))
	defer srv.Close()
	svc := &PRService{
		Cache: newTestCoordinator(t),
		REST:  &rest.Client{BaseURL: srv.URL, Token: "tok"},
		Owner: "owner", Repo: "repo",
		Log: pholog.NewNop(), Now: func() time.Time { return frozenNow },
	}
	_, _, err := svc.LoadDiff(context.Background(), testRepo(), 42, "abc", false)
	if !errors.Is(err, rest.ErrDiffTooLarge) {
		t.Fatalf("err = %v, want ErrDiffTooLarge", err)
	}
}
