package model

// DiffModel is the fully-parsed representation of a unified diff.
type DiffModel struct {
	Repo     string
	PRNumber int
	BaseSHA  string // full 40-char SHA, populated by the service layer
	HeadSHA  string // full 40-char SHA, populated by the service layer
	Files    []DiffFile
	// FileIndex maps NewPath -> index in Files slice for O(1) lookup.
	FileIndex map[string]int
	Stats     DiffStats
}

// DiffStats aggregates counts across all files in a DiffModel.
type DiffStats struct {
	TotalAdditions int
	TotalDeletions int
	TotalFiles     int
	TotalHunks     int
}

// DiffFile represents a single changed file in a unified diff.
type DiffFile struct {
	OldPath   string
	NewPath   string
	Status    string // "added", "modified", "removed", "renamed"
	IsBinary  bool
	Additions int
	Deletions int
	Hunks     []DiffHunk

	// DisplayRows is the total number of display rows this file contributes
	// to the content viewport (1 file header + sum of all hunk display rows).
	// Used for file-level virtualization.
	DisplayRows int

	// StartRow is the cumulative sum of DisplayRows of all preceding files.
	// Used for file-level virtualization and search scroll calculations.
	StartRow int
}

// DiffHunk represents one @@ -oldStart,oldCount +newStart,newCount @@ block.
type DiffHunk struct {
	Header   string // e.g. "@@ -1,5 +1,8 @@"
	OldStart int
	OldCount int
	NewStart int
	NewCount int
	Lines    []DiffLine
}

// DiffLine represents a single line within a hunk.
type DiffLine struct {
	Kind    string // "context", "addition", "deletion", "hunk_header", "file_header"
	OldLine *int   // nil for additions
	NewLine *int   // nil for deletions
	Raw     string
	// Anchors are generated per line for GitHub URL construction.
	Anchors []LineAnchor
}

// LineAnchor provides the data needed to construct a GitHub permalink
// to a specific line in a specific file on a specific commit.
type LineAnchor struct {
	Path      string
	CommitSHA string
	Side      string // "LEFT" for deletions, "RIGHT" for additions and context
	Line      *int   // the line number on the appropriate side
}

// LineDisplayRow returns the display row index of a line within this file,
// given the line's index among all lines across all hunks.
// The flatLineIndex is the index across all DiffLine entries in all Hunks
// (i.e. the index used by DiffSearchIndex.Match.LineIndex).
func (f *DiffFile) LineDisplayRow(flatLineIndex int) int {
	displayRow := 1 // 1 for the file header row
	accumulated := 0
	for _, hunk := range f.Hunks {
		// 1 row for the hunk header
		hunkLineCount := len(hunk.Lines)
		if flatLineIndex >= accumulated && flatLineIndex < accumulated+hunkLineCount {
			// The match is in this hunk.
			lineOffsetWithinHunk := flatLineIndex - accumulated
			return displayRow + 1 /* hunk header */ + lineOffsetWithinHunk
		}
		accumulated += hunkLineCount
		displayRow += 1 /* hunk header */ + hunkLineCount
	}
	return displayRow
}
