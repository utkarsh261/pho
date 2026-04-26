package prdetail

import (
	"strings"
)

// descriptionLines returns the display lines for the Description section.
// Returns nil (RowCount = 0) when no body is available or body is empty.
func (m *PRDetailModel) descriptionLines(contentWidth int) []string {
	if m.Detail == nil {
		if m.DetailLoading {
			return []string{"Loading…"}
		}
		return nil
	}
	body := strings.TrimSpace(m.Detail.BodyExcerpt)
	if body == "" {
		return nil // empty description → section omitted
	}

	cw := max(contentWidth, 1)
	var lines []string

	// Section header: bold + muted
	header := "Description"
	if m.theme != nil {
		header = m.theme.MutedTxt.Bold(true).Render(header)
	}
	lines = append(lines, header)

	// Divider rule — muted color
	divider := strings.Repeat("─", cw)
	if m.theme != nil {
		divider = m.theme.MutedTxt.Render(divider)
	}
	lines = append(lines, divider)

	// Markdown-rendered body
	if m.mdRenderer != nil {
		lines = append(lines, m.mdRenderer.Render(body, cw)...)
	} else {
		lines = append(lines, wrapParagraph(body, cw)...)
	}
	return lines
}

// renderDescriptionTab renders the Description tab content at the given scroll
// and viewport dimensions. Returns exactly contentH lines (blank-padded).
func (m *PRDetailModel) renderDescriptionTab(scroll, contentH, contentWidth int) []string {
	lines := m.descriptionLines(contentWidth)
	blank := strings.Repeat(" ", max(contentWidth, 0))
	out := make([]string, contentH)
	for i := range contentH {
		idx := scroll + i
		if idx >= 0 && idx < len(lines) {
			out[i] = lines[idx]
		} else {
			out[i] = blank
		}
	}
	return out
}
