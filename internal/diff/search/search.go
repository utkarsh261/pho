package search

import (
	"strings"

	"github.com/utk/git-term/internal/diff/model"
)

type SearchMatch struct {
	// FileIndex is the index of the file in DiffModel.Files.
	FileIndex int
	// LineIndex is the flat index of the line across all lines in all hunks
	// of all files in the DiffModel (i.e. the cumulative line index).
	LineIndex int
	// StartCol and EndCol are the byte offsets of the match within the
	// line's Raw content (0-indexed, EndCol is exclusive).
	StartCol int
	EndCol   int
}

// DiffSearchIndex is a case-insensitive substring search index over
// all diff content in a DiffModel. It indexes line content only —
// not file paths.
type DiffSearchIndex struct {
	// lines maps flat line index -> line content (lowercased for search).
	lines map[int]string
	// totalLines is the total number of indexable lines.
	totalLines int
	// flatToFile maps flat line index -> file index.
	flatToFile map[int]int
}

func Build(dm *model.DiffModel) *DiffSearchIndex {
	idx := &DiffSearchIndex{
		lines:      make(map[int]string),
		flatToFile: make(map[int]int),
	}

	if dm == nil {
		return idx
	}

	flatIdx := 0
	for fi, file := range dm.Files {
		for _, hunk := range file.Hunks {
			for _, line := range hunk.Lines {
				idx.lines[flatIdx] = strings.ToLower(line.Raw)
				idx.flatToFile[flatIdx] = fi
				flatIdx++
			}
		}
	}
	idx.totalLines = flatIdx
	return idx
}

func (idx *DiffSearchIndex) Search(query string) []SearchMatch {
	if query == "" || idx == nil || idx.totalLines == 0 {
		return nil
	}
	q := strings.ToLower(query)
	var matches []SearchMatch

	for lineIdx := 0; lineIdx < idx.totalLines; lineIdx++ {
		content, ok := idx.lines[lineIdx]
		if !ok {
			continue
		}
		// Find all occurrences of the query in this line.
		start := 0
		for {
			pos := strings.Index(content[start:], q)
			if pos < 0 {
				break
			}
			matches = append(matches, SearchMatch{
				FileIndex: idx.flatToFile[lineIdx],
				LineIndex: lineIdx,
				StartCol:  start + pos,
				EndCol:    start + pos + len(q),
			})
			start += pos + 1
		}
	}
	return matches
}

// NextMatch returns the next match after the current cursor position in the
// given matches slice, wrapping at boundaries. Returns (matchIndex, found).
// The caller uses the returned matchIndex to index into the matches slice
// and get the actual Match with FileIndex/LineIndex.
func NextMatch(matches []SearchMatch, cursor int) (int, bool) {
	if len(matches) == 0 {
		return 0, false
	}
	next := cursor + 1
	if next >= len(matches) {
		next = 0 // wrap
	}
	return next, true
}

// PrevMatch returns the previous match before the current cursor position in the
// given matches slice, wrapping at boundaries. Returns (matchIndex, found).
func PrevMatch(matches []SearchMatch, cursor int) (int, bool) {
	if len(matches) == 0 {
		return 0, false
	}
	prev := cursor - 1
	if prev < 0 {
		prev = len(matches) - 1 // wrap
	}
	return prev, true
}
