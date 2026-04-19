package prdetail

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	diffsearch "github.com/utkarsh261/pho/internal/diff/search"
)

type searchWordMatch struct {
	start   int
	end     int
	current bool
}

type searchRenderContext struct {
	enabled    bool
	current    diffsearch.Match
	lineRanges map[int][]searchWordMatch
}

func (m *PRDetailModel) buildSearchRenderContext() searchRenderContext {
	if !m.searchActive || len(m.searchMatches) == 0 {
		return searchRenderContext{}
	}

	current, ok := m.currentSearchMatch()
	if !ok {
		return searchRenderContext{}
	}

	lineRanges := make(map[int][]searchWordMatch, len(m.searchMatches))
	for i, mt := range m.searchMatches {
		lineRanges[mt.LineIndex] = append(lineRanges[mt.LineIndex], searchWordMatch{
			start:   mt.StartCol,
			end:     mt.EndCol,
			current: i == m.searchCursor,
		})
	}

	return searchRenderContext{
		enabled:    true,
		current:    current,
		lineRanges: lineRanges,
	}
}

func (m *PRDetailModel) searchHighlightStyles() (currentWord, otherWord lipgloss.Style, currentLineBg lipgloss.Color) {
	currentWord = lipgloss.NewStyle().
		Background(lipgloss.Color("#F59E0B")).
		Foreground(lipgloss.Color("#0F172A")).
		Bold(true)
	otherWord = lipgloss.NewStyle().
		Background(lipgloss.Color("#22D3EE")).
		Foreground(lipgloss.Color("#0F172A")).
		Bold(true)
	return currentWord, otherWord, lipgloss.Color("#1D4ED8")
}

func (m *PRDetailModel) renderSearchMatchLine(
	raw string,
	fileIndex int,
	globalLineIndex int,
	ctx searchRenderContext,
	baseStyle lipgloss.Style,
	currentWordStyle lipgloss.Style,
	otherWordStyle lipgloss.Style,
	currentLineBg lipgloss.Color,
) string {
	lineStyle := baseStyle
	if ctx.enabled && fileIndex == ctx.current.FileIndex && globalLineIndex == ctx.current.LineIndex {
		lineStyle = lineStyle.Background(currentLineBg)
	}

	if !ctx.enabled {
		return lineStyle.Render(raw)
	}

	ranges := ctx.lineRanges[globalLineIndex]
	if len(ranges) == 0 {
		return lineStyle.Render(raw)
	}

	data := []byte(raw)
	cursor := 0
	var out strings.Builder

	for _, rg := range ranges {
		start := clamp(rg.start, 0, len(data))
		end := clamp(rg.end, 0, len(data))
		if end < start {
			end = start
		}
		if start < cursor {
			start = cursor
		}
		if end <= start {
			continue
		}

		if start > cursor {
			out.WriteString(lineStyle.Render(string(data[cursor:start])))
		}

		segment := string(data[start:end])
		if rg.current {
			out.WriteString(currentWordStyle.Render(segment))
		} else {
			out.WriteString(otherWordStyle.Render(segment))
		}
		cursor = end
	}

	if cursor < len(data) {
		out.WriteString(lineStyle.Render(string(data[cursor:])))
	}

	return out.String()
}
