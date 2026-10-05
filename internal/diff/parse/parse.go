package parse

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/utkarsh261/pho/internal/diff/model"
)

// PatchUnavailableMarker starts a header line ("pho-patch-unavailable +A -D")
// marking a file whose patch GitHub did not send. Git never writes such a
// line, so it cannot appear in a real diff.
const PatchUnavailableMarker = "pho-patch-unavailable "

// Parse parses a unified diff. DiffLine.Raw values are substrings of raw, so
// the parsed model shares raw's memory instead of copying every line.
func Parse(raw string) (*model.DiffModel, error) {
	dm := &model.DiffModel{
		FileIndex: make(map[string]int),
	}

	if strings.TrimSpace(raw) == "" {
		return dm, nil
	}

	// Split on "diff --git" lines into per-file blocks. Every "\n"-separated
	// element of raw is a line, including the empty element after a trailing
	// newline; lines before the first "diff --git" form a block of their own.
	var block []string
	flush := func() {
		if len(block) == 0 {
			return
		}
		file := parseFileBlock(block)
		fileIndex := len(dm.Files)
		dm.Files = append(dm.Files, file)
		if file.NewPath != "" {
			dm.FileIndex[file.NewPath] = fileIndex
		}
		if file.OldPath != "" && file.NewPath != file.OldPath {
			dm.FileIndex[file.OldPath] = fileIndex
		}
		block = block[:0]
	}
	for rest := raw; ; {
		line, tail, more := strings.Cut(rest, "\n")
		if strings.HasPrefix(line, "diff --git ") {
			flush()
		}
		block = append(block, line)
		if !more {
			break
		}
		rest = tail
	}
	flush()

	computeStats(dm)
	return dm, nil
}

// parseFileBlock parses the lines of one "diff --git ..." block into a DiffFile.
func parseFileBlock(lines []string) model.DiffFile {
	f := model.DiffFile{Status: "modified"}

	inHunk := false
	var currentHunk *model.DiffHunk

	for _, line := range lines {
		line = strings.TrimSuffix(line, "\r")

		// pho writes this marker when converting per-file API results for a
		// file GitHub sent without a patch.
		if !inHunk && strings.HasPrefix(line, PatchUnavailableMarker) {
			f.PatchUnavailable = true
			_, _ = fmt.Sscanf(strings.TrimPrefix(line, PatchUnavailableMarker), "+%d -%d", &f.Additions, &f.Deletions)
			continue
		}

		// Detect binary diff marker.
		if strings.Contains(line, "Binary files ") || strings.Contains(line, "Binary files differ") {
			f.IsBinary = true
			continue
		}

		// Parse --- and +++ file headers.
		if strings.HasPrefix(line, "--- ") {
			path := strings.TrimPrefix(line, "--- ")
			if path == "/dev/null" {
				f.Status = "added"
			} else {
				f.OldPath = cleanPath(path)
			}
			continue
		}
		if strings.HasPrefix(line, "+++ ") {
			path := strings.TrimPrefix(line, "+++ ")
			if path == "/dev/null" {
				f.Status = "removed"
			} else {
				f.NewPath = cleanPath(path)
			}
			continue
		}

		// Detect renamed file from "rename from/to" lines.
		if strings.HasPrefix(line, "rename from ") {
			f.OldPath = strings.TrimPrefix(line, "rename from ")
			f.Status = "renamed"
			continue
		}
		if strings.HasPrefix(line, "rename to ") {
			f.NewPath = strings.TrimPrefix(line, "rename to ")
			f.Status = "renamed"
			continue
		}

		// Parse hunk header: @@ -oldStart,oldCount +newStart,newCount @@
		if strings.HasPrefix(line, "@@") {
			if currentHunk != nil {
				f.Hunks = append(f.Hunks, *currentHunk)
			}
			h := parseHunkHeader(line)
			currentHunk = &h
			inHunk = true
			continue
		}

		// Parse diff content lines (within a hunk).
		if inHunk && currentHunk != nil {
			if len(line) == 0 {
				// Empty line in diff — treat as context with empty content.
				currentHunk.Lines = append(currentHunk.Lines, model.DiffLine{Kind: "context", Raw: ""})
				continue
			}
			content := line[1:]
			var dl model.DiffLine
			switch line[0] {
			case ' ':
				dl = model.DiffLine{Kind: "context", Raw: content}
			case '+':
				dl = model.DiffLine{Kind: "addition", Raw: content}
				f.Additions++
			case '-':
				dl = model.DiffLine{Kind: "deletion", Raw: content}
				f.Deletions++
			case '\\':
				// "\ No newline at end of file" — skip as a metadata line.
				continue
			default:
				// Unknown prefix — treat as context.
				dl = model.DiffLine{Kind: "context", Raw: line}
			}
			f.MaxLineLen = max(f.MaxLineLen, len(dl.Raw))
			currentHunk.Lines = append(currentHunk.Lines, dl)
		}
	}

	// Flush last hunk.
	if currentHunk != nil {
		f.Hunks = append(f.Hunks, *currentHunk)
	}

	// Populate line numbers for context/addition/deletion lines.
	populateLineNumbers(&f)

	// Compute display rows for virtualization.
	f.DisplayRows = fileDisplayRows(&f)

	// Set default paths if not yet set, from "diff --git a/path b/path".
	if f.NewPath == "" && f.OldPath == "" {
		firstLine := strings.TrimSpace(lines[0])
		if idx := strings.Index(firstLine, "diff --git "); idx == 0 {
			rest := strings.TrimPrefix(firstLine, "diff --git ")
			parts := strings.SplitN(rest, " ", 2)
			if len(parts) == 2 {
				f.OldPath = cleanPath(parts[0])
				f.NewPath = cleanPath(parts[1])
			} else if len(parts) == 1 {
				f.OldPath = cleanPath(parts[0])
				f.NewPath = cleanPath(parts[0])
			}
		}
	}
	if f.NewPath == "" {
		f.NewPath = f.OldPath
	}
	if f.OldPath == "" {
		f.OldPath = f.NewPath
	}

	return f
}

// cleanPath strips the a/ or b/ prefix that git adds to paths.
func cleanPath(p string) string {
	p = strings.TrimSpace(p)
	if len(p) >= 2 && (p[0] == 'a' || p[0] == 'b') && p[1] == '/' {
		return p[2:]
	}
	return p
}

// parseHunkHeader parses "@@ -oldStart,oldCount +newStart,newCount @@ ..." into a DiffHunk.
func parseHunkHeader(line string) model.DiffHunk {
	h := model.DiffHunk{Header: line}

	// Find all @@ markers.
	endIdx := strings.Index(line[2:], "@@")
	if endIdx < 0 {
		return h
	}
	rangeStr := strings.TrimSpace(line[2 : 2+endIdx])

	// Parse old and new ranges.
	parts := strings.Fields(rangeStr)
	for _, part := range parts {
		if strings.HasPrefix(part, "-") {
			h.OldStart, h.OldCount = parseRange(part[1:])
		} else if strings.HasPrefix(part, "+") {
			h.NewStart, h.NewCount = parseRange(part[1:])
		}
	}
	return h
}

// parseRange parses "start" or "start,count" and returns (start, count).
func parseRange(s string) (start, count int) {
	parts := strings.Split(s, ",")
	if len(parts) == 0 {
		return 0, 0
	}
	start, _ = strconv.Atoi(parts[0])
	count = 1
	if len(parts) > 1 {
		count, _ = strconv.Atoi(parts[1])
	}
	return start, count
}

// populateLineNumbers assigns OldLine and NewLine pointers to each DiffLine.
func populateLineNumbers(f *model.DiffFile) {
	for i := range f.Hunks {
		hunk := &f.Hunks[i]

		ol := hunk.OldStart
		nl := hunk.NewStart

		for j := range hunk.Lines {
			dl := &hunk.Lines[j]
			switch dl.Kind {
			case "context":
				oldPtr := ol
				newPtr := nl
				dl.OldLine = &oldPtr
				dl.NewLine = &newPtr
				ol++
				nl++
			case "addition":
				newPtr := nl
				dl.NewLine = &newPtr
				nl++
			case "deletion":
				oldPtr := ol
				dl.OldLine = &oldPtr
				ol++
			}
		}
	}
}

// fileDisplayRows computes the number of display rows for this file.
// 1 row for the file header + 1 row per hunk header + 1 row per diff line.
func fileDisplayRows(f *model.DiffFile) int {
	rows := 1 // file header
	for _, hunk := range f.Hunks {
		rows++ // hunk header
		rows += len(hunk.Lines)
	}
	return rows
}

// computeStats populates DiffModel.Stats from the file list.
func computeStats(dm *model.DiffModel) {
	dm.Stats.TotalFiles = len(dm.Files)
	for _, f := range dm.Files {
		dm.Stats.TotalAdditions += f.Additions
		dm.Stats.TotalDeletions += f.Deletions
		dm.Stats.TotalHunks += len(f.Hunks)
	}
}
