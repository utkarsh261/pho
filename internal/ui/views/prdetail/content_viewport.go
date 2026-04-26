package prdetail

import (
	"fmt"
	"strings"
	"time"

	"github.com/utkarsh261/pho/internal/domain"
)

// buildInlineBody formats a slice of inline comments into a markdown string
// used as the body of a review entry when the reviewer left no summary text.
func buildInlineBody(comments []domain.PreviewInlineComment) string {
	parts := make([]string, 0, len(comments))
	for _, c := range comments {
		if c.Body == "" {
			continue
		}
		loc := c.Path
		if c.Line > 0 {
			loc = fmt.Sprintf("%s:%d", c.Path, c.Line)
		}
		parts = append(parts, fmt.Sprintf("**%s**\n%s", loc, c.Body))
	}
	return strings.Join(parts, "\n\n")
}

func relativeTime(t time.Time) string {
	age := time.Since(t)
	switch {
	case age < time.Minute:
		return "just now"
	case age < time.Hour:
		return fmt.Sprintf("%dm", int(age.Minutes()))
	case age < 24*time.Hour:
		return fmt.Sprintf("%dh", int(age.Hours()))
	case age < 3*24*time.Hour:
		return fmt.Sprintf("%dd", int(age.Hours()/24))
	default:
		if t.Year() == time.Now().Year() {
			return t.Format("Jan 02")
		}
		return t.Format("Jan 02 2006")
	}
}

// ContentSection holds the display-row range for one section of the unified content viewport.
// Sections are always ordered: Description → Diff → Comments.
type ContentSection struct {
	Section  domain.PRDetailSection // SectionDescription, SectionDiff, or SectionComments
	StartRow int                    // first absolute display row in the viewport
	RowCount int                    // number of display rows; 0 means the section is skipped
}

// buildContentSections precomputes the section slice based on the current model state.
// contentWidth is the usable text-column width inside the content area.
//
// Rules:
//   - Description RowCount = 0 when no body is available (section visually absent).
//   - Comments section is omitted entirely when there are no review nodes.
//   - All other sections are always present (Diff always ≥ 1 row when loaded or loading).
func (m *PRDetailModel) buildContentSections(contentWidth int) []ContentSection {
	cursor := 0

	// ── Description ──────────────────────────────────────────────────────────
	descLines := m.descriptionLines(contentWidth)
	desc := ContentSection{
		Section:  domain.SectionDescription,
		StartRow: cursor,
		RowCount: len(descLines),
	}
	cursor += len(descLines)

	// ── Diff ─────────────────────────────────────────────────────────────────
	diffCount := m.diffSectionRowCount()
	diff := ContentSection{
		Section:  domain.SectionDiff,
		StartRow: cursor,
		RowCount: diffCount,
	}
	cursor += diffCount

	sections := []ContentSection{desc, diff}

	// ── Comments ─────────────────────────────────────────────────────────────
	// Always present when detail is loaded (shows "No reviews" when empty).
	// Omitted only while detail is still loading.
	// Row count for sections uses -1 (no active entry) so it stays stable regardless
	// of cursor position — active highlight doesn't change the row count.
	cLines := m.commentLines(contentWidth, -1)
	if len(cLines) > 0 {
		sections = append(sections, ContentSection{
			Section:  domain.SectionComments,
			StartRow: cursor,
			RowCount: len(cLines),
		})
	}

	return sections
}

// totalRowsInSections returns the sum of all section RowCounts.
func totalRowsInSections(sections []ContentSection) int {
	n := 0
	for _, s := range sections {
		n += s.RowCount
	}
	return n
}

// activeSectionAt returns the section whose range contains scroll.
// Used by the scroll-spy tab indicators to decide which label to highlight.
func activeSectionAt(sections []ContentSection, scroll int) domain.PRDetailSection {
	active := domain.SectionDescription
	for _, s := range sections {
		if s.RowCount == 0 {
			continue
		}
		if scroll >= s.StartRow {
			active = s.Section
		}
	}
	return active
}

// findSection returns the ContentSection for target and whether it has RowCount > 0.
func findSection(sections []ContentSection, target domain.PRDetailSection) (ContentSection, bool) {
	for _, s := range sections {
		if s.Section == target {
			return s, s.RowCount > 0
		}
	}
	return ContentSection{}, false
}

// renderContentLines renders the visible content lines using the overscan algorithm.
// Returns exactly contentH lines (blank-padded as needed).
//
// overscan = 100 rows pre-rendered on each side of the visible window prevents blank
// flicker when the user scrolls quickly.
func (m *PRDetailModel) renderContentLines(
	sections []ContentSection, scroll, contentH, contentWidth int,
) []string {
	total := totalRowsInSections(sections)

	if total == 0 {
		// All sections empty → centered "No content" message.
		out := make([]string, contentH)
		mid := contentH / 2
		msg := "No content"
		if m.theme != nil {
			msg = m.theme.MutedTxt.Render(msg)
		}
		blank := strings.Repeat(" ", max(contentWidth, 0))
		for i := range out {
			if i == mid {
				out[i] = msg
			} else {
				out[i] = blank
			}
		}
		return out
	}

	const overscan = 100
	renderStart := max(0, scroll-overscan)
	renderEnd := min(total, scroll+contentH+overscan)

	// Pre-compute source lines once (cheap for desc + comments).
	descLines := m.descriptionLines(contentWidth)
	cLines := m.commentLines(contentWidth, m.commentCursor)

	// collected maps absolute row index → rendered string.
	collected := make(map[int]string, contentH+overscan*2)

	for _, sec := range sections {
		if sec.RowCount == 0 {
			continue
		}
		secEnd := sec.StartRow + sec.RowCount
		overlapStart := max(sec.StartRow, renderStart)
		overlapEnd := min(secEnd, renderEnd)
		if overlapStart >= overlapEnd {
			continue
		}
		localStart := overlapStart - sec.StartRow
		localEnd := overlapEnd - sec.StartRow

		switch sec.Section {
		case domain.SectionDescription:
			for i := localStart; i < localEnd; i++ {
				if i < len(descLines) {
					collected[sec.StartRow+i] = descLines[i]
				}
			}
		case domain.SectionDiff:
			diffLines := m.renderDiffSectionLines(localStart, localEnd, contentWidth)
			for i, line := range diffLines {
				collected[sec.StartRow+localStart+i] = line
			}
		case domain.SectionComments:
			for i := localStart; i < localEnd; i++ {
				if i < len(cLines) {
					collected[sec.StartRow+i] = cLines[i]
				}
			}
		}
	}

	blank := strings.Repeat(" ", max(contentWidth, 0))
	out := make([]string, contentH)
	for i := range contentH {
		if line, ok := collected[scroll+i]; ok {
			out[i] = line
		} else {
			out[i] = blank
		}
	}
	return out
}
