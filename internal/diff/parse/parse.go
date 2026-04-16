package parse

import (
	"bufio"
	"strconv"
	"strings"

	"github.com/utkarsh261/pho/internal/diff/model"
)

func Parse(raw string) (*model.DiffModel, error) {
	dm := &model.DiffModel{
		FileIndex: make(map[string]int),
	}

	if strings.TrimSpace(raw) == "" {
		return dm, nil
	}

	// Split on "diff --git" to get per-file sections.
	parts := splitOnDiffGit(raw)
	for _, part := range parts {
		file, err := parseFileBlock(part)
		if err != nil {
			return nil, err
		}
		fileIndex := len(dm.Files)
		dm.Files = append(dm.Files, file)
		if file.NewPath != "" {
			dm.FileIndex[file.NewPath] = fileIndex
		}
		if file.OldPath != "" && file.NewPath != file.OldPath {
			dm.FileIndex[file.OldPath] = fileIndex
		}
	}

	computeStats(dm)
	return dm, nil
}

// splitOnDiffGit splits the raw diff on "diff --git" markers.
func splitOnDiffGit(raw string) []string {
	lines := strings.Split(raw, "\n")
	var parts []string
	var current strings.Builder

	for _, line := range lines {
		if strings.HasPrefix(line, "diff --git ") {
			if current.Len() > 0 {
				parts = append(parts, current.String())
			}
			current.Reset()
		}
		current.WriteString(line)
		current.WriteString("\n")
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	return parts
}

// parseFileBlock parses one "diff --git ..." block into a DiffFile.
func parseFileBlock(block string) (model.DiffFile, error) {
	f := model.DiffFile{Status: "modified"}

	scanner := bufio.NewScanner(strings.NewReader(block))
	var hunkLines []string
	inHunk := false
	var currentHunk *model.DiffHunk

	for scanner.Scan() {
		line := scanner.Text()

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

		// Detect renamed file from "similarity index" or "rename from/to" lines.
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
			// Flush previous hunk.
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
				dl := model.DiffLine{Kind: "context", Raw: ""}
				currentHunk.Lines = append(currentHunk.Lines, dl)
				continue
			}
			prefix := line[0]
			content := ""
			if len(line) > 1 {
				content = line[1:]
			}

			var dl model.DiffLine
			switch prefix {
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
			currentHunk.Lines = append(currentHunk.Lines, dl)

			// Accumulate hunk lines for later display row computation.
			hunkLines = append(hunkLines, line)
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

	// Set default paths if not yet set (from the diff --git line).
	if f.NewPath == "" && f.OldPath == "" {
		// Try to extract from the first line of the block.
		firstLine := strings.TrimSpace(strings.Split(block, "\n")[0])
		// "diff --git a/path b/path"
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

	return f, nil
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
