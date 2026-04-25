package prdetail

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	diffmodel "github.com/utkarsh261/pho/internal/diff/model"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/ui/theme"
)

// This is an internal enum — NOT a domain.FocusTarget.
type PRDetailFocus int

const (
	// FocusFiles means the file list in the left panel has focus.
	FocusFiles PRDetailFocus = iota
	// FocusCI means the CI check list in the left panel has focus.
	FocusCI
	// FocusContent means the right content viewport has focus.
	FocusContent
)

// State mutations are driven entirely by PRDetailModel.Update() — this struct
// intentionally has no Update() method.
type LeftPanelModel struct {
	// Data — set by PRDetailModel when diff/detail data arrive
	Files   []diffmodel.DiffFile
	Checks  []domain.PreviewCheckRow
	Loading bool // true while the diff is still being fetched

	// Navigation state — mutated by PRDetailModel.Update
	FilesScroll int // index of first visible file row
	CIScroll    int // index of first visible CI row
	FileIndex   int // index of the highlighted (cursor) file

	Focus PRDetailFocus

	theme *theme.Theme
}

// SetTheme wires the theme into the left panel.
func (m *LeftPanelModel) SetTheme(th *theme.Theme) { m.theme = th }

// View renders the left panel into a string of LeftPanelWidth columns.
// height is the total outer row count available for the left panel.
// spinnerFrame is the current animation frame from PRDetailModel's spinner.
func (m *LeftPanelModel) View(height int, spinnerFrame string) string {
	if height <= 0 {
		return ""
	}
	ciH := computeCIHeight(height, len(m.Checks))
	filesH := max(height-ciH, 5)

	filesView := m.renderFilesArea(filesH, spinnerFrame)
	if ciH > 0 {
		ciView := m.renderCIArea(ciH)
		return lipgloss.JoinVertical(lipgloss.Left, filesView, ciView)
	}
	return filesView
}

// outerHeight includes the top and bottom border rows.
func (m *LeftPanelModel) renderFilesArea(outerHeight int, spinnerFrame string) string {
	borderColor := m.borderColorFor(FocusFiles)
	// subtract top border, title, mid border, bottom border
	innerH := max(outerHeight-4, 1)

	tabLabel := "FILES"
	if m.theme != nil {
		if m.Focus == FocusFiles {
			tabLabel = m.theme.TabActive.Render(tabLabel)
		} else {
			tabLabel = m.theme.TabInactive.Render(tabLabel)
		}
	} else {
		if m.Focus == FocusFiles {
			tabLabel = "[" + tabLabel + "]"
		}
	}

	var rows []string
	if m.Loading {
		rows = m.spinnerRows(innerH, spinnerFrame)
	} else {
		rows = m.fileRows(innerH)
	}

	headBox := lipgloss.NewStyle().
		Border(panelHeadBorder).
		BorderForeground(borderColor).
		Padding(0, 1).
		Width(LeftPanelWidth - 2). // Using LeftPanelWidth - 2 matches the exact outer inner width
		Render(tabLabel)

	bodyBox := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderTop(false).
		BorderForeground(borderColor).
		Padding(0, 1).
		Width(LeftPanelWidth - 2).
		Height(innerH).
		Render(strings.Join(rows, "\n"))

	return lipgloss.JoinVertical(lipgloss.Left, headBox, bodyBox)
}

// spinnerRows returns innerH rows with a centered spinner in the middle row.
func (m *LeftPanelModel) spinnerRows(innerH int, spinnerFrame string) []string {
	rows := make([]string, innerH)
	midRow := innerH / 2
	centerStyle := lipgloss.NewStyle().Width(LeftPanelWidth - 4).Align(lipgloss.Center)

	for i := range rows {
		if i == midRow {
			frame := spinnerFrame
			if m.theme != nil {
				frame = m.theme.CIPending.Render(spinnerFrame)
			}
			rows[i] = centerStyle.Render(frame)
		} else {
			rows[i] = ""
		}
	}
	return rows
}

// fileRows returns up to innerH rendered file rows, respecting FilesScroll, with double spacing.
func (m *LeftPanelModel) fileRows(innerH int) []string {
	if len(m.Files) == 0 {
		rows := make([]string, innerH)
		msg := "no files"
		if m.theme != nil {
			msg = m.theme.MutedTxt.Render(msg)
		}
		centerStyle := lipgloss.NewStyle().Width(LeftPanelWidth - 4).Align(lipgloss.Center)
		rows[0] = centerStyle.Render(msg)
		for i := 1; i < innerH; i++ {
			rows[i] = ""
		}
		return rows
	}

	visibleItems := max(innerH, 1)

	scroll := clamp(m.FilesScroll, 0, max(0, len(m.Files)-visibleItems))
	var rows []string

	for i := range visibleItems {
		fileIdx := scroll + i
		if fileIdx >= len(m.Files) {
			break
		}
		rows = append(rows, m.renderFileRow(m.Files[fileIdx], fileIdx))
	}

	for len(rows) < innerH {
		rows = append(rows, "")
	}
	if len(rows) > innerH {
		rows = rows[:innerH]
	}
	return rows
}

// renderFileRow renders a single file entry as exactly lpInner visible columns.
// Selected rows (when Files is focused) use a full-width background highlight
// matching the command palette's selection UX.
func (m *LeftPanelModel) renderFileRow(f diffmodel.DiffFile, idx int) string {
	isSelected := idx == m.FileIndex && m.Focus == FocusFiles

	path := truncatePathLeft(f.NewPath, lpPathMax) // exactly lpPathMax visible chars

	var stats string
	if isSelected && m.theme != nil {
		// Plain stats so the selected-row style controls foreground uniformly.
		stats = formatFileStats(f.Additions, f.Deletions)
	} else if m.theme != nil {
		stats = formatFileStatsColored(f.Additions, f.Deletions, m.theme)
	} else {
		stats = formatFileStats(f.Additions, f.Deletions)
	}

	// 2-char left padding, matching the command palette row layout.
	content := "  " + path + stats

	if isSelected {
		if m.theme != nil {
			return m.theme.ListSelected.Width(lpInner).Render(content)
		}
		return lipgloss.NewStyle().Reverse(true).Width(lpInner).Render(content)
	}

	// Use dashboard's fitLine to strictly prevent random word wrapping on borders.
	return fitLine(content, lpInner)
}

// formatFileStatsColored returns a stats string of exactly lpStatsWidth visible chars
// with additions rendered in green and deletions in red.
func formatFileStatsColored(additions, deletions int, th *theme.Theme) string {
	addPart := fmt.Sprintf("+%d", additions)
	delPart := fmt.Sprintf("-%d", deletions)
	visibleLen := len([]rune(addPart)) + 1 + len([]rune(delPart)) // "+N -N" visible rune count
	budget := lpStatsWidth - 1                                    // exclude leading space
	if visibleLen > budget {
		// Falls back to plain (no color) when numbers are too large to fit.
		return formatFileStats(additions, deletions)
	}
	colored := th.Additions.Render(addPart) + " " + th.Deletions.Render(delPart)
	return lipgloss.NewStyle().Width(lpStatsWidth).Align(lipgloss.Right).Render(colored)
}

func (m *LeftPanelModel) renderCIArea(outerHeight int) string {
	borderColor := m.borderColorFor(FocusCI)
	// subtract top, title, mid, bottom borders
	innerH := max(outerHeight-4, 1)

	tabLabel := "CI"
	if m.theme != nil {
		if m.Focus == FocusCI {
			tabLabel = m.theme.TabActive.Render(tabLabel)
		} else {
			tabLabel = m.theme.TabInactive.Render(tabLabel)
		}
	} else {
		if m.Focus == FocusCI {
			tabLabel = "[" + tabLabel + "]"
		}
	}

	visibleItems := max(innerH, 1)

	scroll := clamp(m.CIScroll, 0, max(0, len(m.Checks)-visibleItems))
	var rows []string
	for i := 0; i < visibleItems; i++ {
		checkIdx := scroll + i
		if checkIdx >= len(m.Checks) {
			break
		}
		rows = append(rows, m.renderCIRow(m.Checks[checkIdx]))
	}
	for len(rows) < innerH {
		rows = append(rows, "")
	}
	if len(rows) > innerH {
		rows = rows[:innerH]
	}

	headBox := lipgloss.NewStyle().
		Border(panelHeadBorder).
		BorderForeground(borderColor).
		Padding(0, 1).
		Width(LeftPanelWidth - 2).
		Render(tabLabel)

	bodyBox := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderTop(false).
		BorderForeground(borderColor).
		Padding(0, 1).
		Width(LeftPanelWidth - 2).
		Height(innerH).
		Render(strings.Join(rows, "\n"))

	return lipgloss.JoinVertical(lipgloss.Left, headBox, bodyBox)
}

func (m *LeftPanelModel) renderCIRow(check domain.PreviewCheckRow) string {
	icon := ciIconChar(check)
	if m.theme != nil {
		icon = ciIconStyled(check, m.theme)
	}

	name := truncatePathLeft(check.Name, lpCINameMax) // exactly lpCINameMax chars
	status := formatCIStatus(check.State)             // exactly lpCIStatusWidth chars

	row := icon + " " + name + " " + status
	return fitLine(row, lpInner)
}

func (m *LeftPanelModel) borderColorFor(target PRDetailFocus) lipgloss.Color {
	focused := m.Focus == target
	if m.theme != nil {
		if focused {
			return m.theme.Primary
		}
		return m.theme.Border
	}
	if focused {
		return theme.Default().Primary
	}
	return theme.Default().Border
}

func ciIconChar(check domain.PreviewCheckRow) string {
	switch check.State {
	case "SUCCESS":
		return "✓"
	case "FAILURE", "ERROR":
		return "✗"
	default:
		return "·"
	}
}

func ciIconStyled(check domain.PreviewCheckRow, th *theme.Theme) string {
	icon := ciIconChar(check)
	switch check.State {
	case "SUCCESS":
		return th.CISuccess.Render(icon)
	case "FAILURE", "ERROR":
		return th.CIFailure.Render(icon)
	default:
		return th.CIPending.Render(icon)
	}
}

func formatCIStatus(state string) string {
	var short string
	switch state {
	case "SUCCESS":
		short = "pass"
	case "FAILURE":
		short = "fail"
	case "ERROR":
		short = "err"
	case "PENDING", "QUEUED", "WAITING":
		short = "pend"
	case "IN_PROGRESS":
		short = "run"
	case "COMPLETED":
		short = "done"
	default:
		short = strings.ToLower(state)
	}
	runes := []rune(short)
	if len(runes) > lpCIStatusWidth {
		runes = append(runes[:lpCIStatusWidth-1], '…')
	}
	return lipgloss.NewStyle().Width(lpCIStatusWidth).Align(lipgloss.Left).Render(string(runes))
}

func centerInWidth(s string, width int) string {
	return lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Render(s)
}
