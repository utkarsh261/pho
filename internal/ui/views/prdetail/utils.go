package prdetail

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/utkarsh261/pho/internal/diff/model"
)

// contentViewportWidth returns the usable text-column width inside the content area
// given the outer right-panel width.
func contentViewportWidth(rightWidth int) int {
	innerW := max(rightWidth-2, 1)
	return max(innerW-2, 1)
}

// no abs for int in standard lib? wth?
func abs(a int) int {
	if a < 0 {
		return -a
	}
	return a
}

// firstDiffCursor returns the (fileIdx, hunkIdx, lineIdx) of the first
// actual diff line in the entire PR, skipping binary files.
func firstDiffCursor(dm *model.DiffModel) (fileIdx, hunkIdx, lineIdx int) {
	if dm == nil {
		return 0, 0, 0
	}
	for fi := range dm.Files {
		f := &dm.Files[fi]
		if f.IsBinary || len(f.Hunks) == 0 {
			continue
		}
		for hi := range f.Hunks {
			if len(f.Hunks[hi].Lines) > 0 {
				return fi, hi, 0
			}
		}
	}
	return 0, 0, 0
}

// lastDiffCursor returns the (fileIdx, hunkIdx, lineIdx) of the last
// actual diff line in the entire PR, skipping binary files.
func lastDiffCursor(dm *model.DiffModel) (fileIdx, hunkIdx, lineIdx int) {
	if dm == nil {
		return 0, 0, 0
	}
	for fi := len(dm.Files) - 1; fi >= 0; fi-- {
		f := &dm.Files[fi]
		if f.IsBinary || len(f.Hunks) == 0 {
			continue
		}
		for hi := len(f.Hunks) - 1; hi >= 0; hi-- {
			h := &f.Hunks[hi]
			if len(h.Lines) > 0 {
				return fi, hi, len(h.Lines) - 1
			}
		}
	}
	return 0, 0, 0
}

// anchorForLine returns the path, line number, and side for a diff line.
// It uses the generated anchor if present, otherwise it infers path, line
// number and side from the line kind and surrounding file so that inline
// comments can be created even when anchors were not populated (e.g. empty
// head SHA on hosts that don't expose headRefOid).
func anchorForLine(file *model.DiffFile, line *model.DiffLine) (path string, lineNum int, side string, ok bool) {
	if len(line.Anchors) > 0 && line.Anchors[0].Side != "" {
		a := line.Anchors[0]
		return a.Path, *a.Line, a.Side, true
	}
	path = file.NewPath
	if file.Status == "removed" {
		path = file.OldPath
	}
	switch line.Kind {
	case "context", "addition":
		if line.NewLine == nil {
			return "", 0, "", false
		}
		return path, *line.NewLine, "RIGHT", true
	case "deletion":
		if line.OldLine == nil {
			return "", 0, "", false
		}
		return path, *line.OldLine, "LEFT", true
	}
	return "", 0, "", false
}

// generateDraftID creates a simple unique ID for a draft comment.
func generateDraftID() string {
	return fmt.Sprintf("draft-%d-%d", time.Now().UnixNano(), rand.Intn(10000))
}

func humanizeMergeState(state string) string {
	switch strings.ToUpper(state) {
	case "CONFLICTING":
		return "conflicting"
	case "BLOCKED":
		return "blocked"
	case "BEHIND":
		return "behind"
	case "HAS_HOOKS":
		return "has hooks"
	case "UNSTABLE":
		return "unstable"
	case "DIRTY":
		return "dirty"
	case "CLEAN":
		return "clean"
	case "":
		return "unknown"
	default:
		return strings.ToLower(state)
	}
}
