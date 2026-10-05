package prdetail

import (
	"unicode"
	"unicode/utf8"

	diffmodel "github.com/utkarsh261/pho/internal/diff/model"
)

// byteRange is a half-open [start, end) byte range within a diff line's Raw text.
type byteRange struct{ start, end int }

// intraLineChanges pairs each run of deletions in a hunk with the run of
// additions that follows it (line i with line i) and returns, per line index,
// the byte range that actually changed. Pairs that share too little context
// to be a real edit get no range, so whole-line rewrites are not highlighted.
func intraLineChanges(lines []diffmodel.DiffLine) map[int]byteRange {
	var out map[int]byteRange
	for i := 0; i < len(lines); {
		if lines[i].Kind != "deletion" {
			i++
			continue
		}
		delStart := i
		for i < len(lines) && lines[i].Kind == "deletion" {
			i++
		}
		addStart := i
		for i < len(lines) && lines[i].Kind == "addition" {
			i++
		}
		n := min(addStart-delStart, i-addStart)
		for k := range n {
			d, a := delStart+k, addStart+k
			dr, ar, ok := changedRanges(lines[d].Raw, lines[a].Raw)
			if !ok {
				continue
			}
			if out == nil {
				out = make(map[int]byteRange)
			}
			out[d], out[a] = dr, ar
		}
	}
	return out
}

// changedRanges returns the differing middle of old and new after removing
// their common prefix and suffix, widened to whole words. ok is false when
// the lines share less than a third of their text (a rewrite, not an edit).
func changedRanges(oldRaw, newRaw string) (oldR, newR byteRange, ok bool) {
	skip := 0
	if len(oldRaw) > 0 && len(newRaw) > 0 && (oldRaw[0] == '-' || oldRaw[0] == '+') && (newRaw[0] == '-' || newRaw[0] == '+') {
		skip = 1 // leading +/- markers always differ
	}
	o, n := oldRaw[skip:], newRaw[skip:]
	p := 0
	for p < len(o) && p < len(n) && o[p] == n[p] {
		p++
	}
	// Back off so the prefix ends on a rune boundary in both strings.
	for p > 0 && (!runeStartAt(o, p) || !runeStartAt(n, p)) {
		p--
	}
	s := 0
	for s < len(o)-p && s < len(n)-p && o[len(o)-1-s] == n[len(n)-1-s] {
		s++
	}
	// Likewise the suffix must start on a rune boundary in both strings.
	for s > 0 && (!runeStartAt(o, len(o)-s) || !runeStartAt(n, len(n)-s)) {
		s--
	}
	if p == len(o) && p == len(n) {
		return byteRange{}, byteRange{}, false // identical text
	}
	shared := p + s
	if shared*3 < max(len(o), len(n)) {
		return byteRange{}, byteRange{}, false
	}
	oStart, oEnd := widenToWord(o, p, len(o)-s)
	nStart, nEnd := widenToWord(n, p, len(n)-s)
	return byteRange{skip + oStart, skip + oEnd}, byteRange{skip + nStart, skip + nEnd}, true
}

// runeStartAt reports whether byte offset i in s begins a UTF-8 rune
// (the end of the string counts as a boundary).
func runeStartAt(s string, i int) bool {
	return i >= len(s) || utf8.RuneStart(s[i])
}

// widenToWord grows [start, end) outward to word boundaries in s.
func widenToWord(s string, start, end int) (int, int) {
	isWord := func(r rune) bool { return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) }
	for start > 0 {
		r, size := utf8.DecodeLastRuneInString(s[:start])
		if !isWord(r) {
			break
		}
		start -= size
	}
	for end < len(s) {
		r, size := utf8.DecodeRuneInString(s[end:])
		if !isWord(r) {
			break
		}
		end += size
	}
	return start, end
}
