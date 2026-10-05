package pr

import (
	"fmt"
	"strings"

	"github.com/utkarsh261/pho/internal/diff/parse"
	"github.com/utkarsh261/pho/internal/github/rest"
)

// maxListedFiles is GitHub's cap on the per-file endpoints.
const maxListedFiles = 3000

// unifiedDiffFromFiles rebuilds a unified diff from per-file API results so
// it goes through the same parser (and line numbering) as a raw diff.
// Files GitHub sent without a patch get a parse.PatchUnavailableMarker line.
func unifiedDiffFromFiles(files []rest.ChangedFile) string {
	var b strings.Builder
	for _, f := range files {
		newPath := f.Filename
		oldPath := newPath
		if f.PreviousFilename != "" {
			oldPath = f.PreviousFilename
		}
		fmt.Fprintf(&b, "diff --git a/%s b/%s\n", oldPath, newPath)
		if oldPath != newPath {
			fmt.Fprintf(&b, "rename from %s\nrename to %s\n", oldPath, newPath)
		}
		if f.Patch == "" && f.Changes > 0 {
			fmt.Fprintf(&b, "%s+%d -%d\n", parse.PatchUnavailableMarker, f.Additions, f.Deletions)
		}
		if f.Patch == "" && f.Status != "added" && f.Status != "removed" {
			continue
		}
		from, to := "a/"+oldPath, "b/"+newPath
		switch f.Status {
		case "added":
			from = "/dev/null"
		case "removed":
			to = "/dev/null"
		}
		fmt.Fprintf(&b, "--- %s\n+++ %s\n", from, to)
		if f.Patch != "" {
			b.WriteString(f.Patch)
			if !strings.HasSuffix(f.Patch, "\n") {
				b.WriteByte('\n')
			}
		}
	}
	return b.String()
}
