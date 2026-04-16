package dashboard

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/ui/theme"
)

type PreviewPanelModel struct {
	Width              int
	Height             int
	Scroll             int
	Loading            bool
	PendingFetch       bool
	DebounceGeneration int
	DebounceDelay      time.Duration

	selectedRepo   string
	selectedNumber int
	summary        *domain.PullRequestSummary
	preview        *domain.PRPreviewSnapshot
	theme          *theme.Theme
	spinner        spinner.Model
	lastKey        string
}

func NewPreviewPanelModel() *PreviewPanelModel {
	s := spinner.New(spinner.WithSpinner(spinner.Points))
	s.Spinner.FPS = time.Millisecond * 8
	return &PreviewPanelModel{
		DebounceDelay: 100 * time.Millisecond,
		spinner:       s,
	}
}

func (m *PreviewPanelModel) Init() tea.Cmd { return m.spinner.Tick }

func (m *PreviewPanelModel) SetRect(width, height int) {
	m.Width = width
	m.Height = height
	m.clampScroll()
}

func (m *PreviewPanelModel) SetTheme(th *theme.Theme) {
	m.theme = th
	if m.theme != nil {
		m.spinner.Style = lipgloss.NewStyle().Foreground(m.theme.Warning)
	}
}

func (m *PreviewPanelModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var spinCmd tea.Cmd
	m.spinner, spinCmd = m.spinner.Update(msg)

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.SetRect(msg.Width, msg.Height)
		return m, spinCmd
	case tea.KeyMsg:
		prevKey := m.lastKey
		if msg.String() != "g" {
			m.lastKey = ""
		}
		switch msg.String() {
		case "j", "down":
			m.Scroll++
			m.clampScroll()
		case "k", "up":
			m.Scroll--
			m.clampScroll()
		case "g":
			if prevKey == "g" {
				m.Scroll = 0
				m.clampScroll()
			} else {
				m.lastKey = "g"
			}
		case "G":
			m.Scroll = len(m.buildLines())
			m.clampScroll()
		case "ctrl+d":
			m.Scroll += m.Height / 2
			m.clampScroll()
		case "ctrl+u":
			m.Scroll -= m.Height / 2
			m.clampScroll()
		}
		return m, spinCmd
	case SelectPRMsg:
		samePR := m.selectedRepo == msg.Repo && m.selectedNumber == msg.Number

		m.selectedRepo = msg.Repo
		m.selectedNumber = msg.Number
		summary := msg.Summary
		m.summary = &summary

		if !samePR {
			m.preview = nil
			m.Scroll = 0
		}

		m.Loading = true
		m.DebounceGeneration++

		if m.PendingFetch && samePR {
			return m, spinCmd
		}
		m.PendingFetch = true
		gen := m.DebounceGeneration
		return m, tea.Batch(spinCmd, tea.Tick(m.DebounceDelay, func(time.Time) tea.Msg {
			return PreviewFetchMsg{Repo: m.selectedRepo, Number: m.selectedNumber, Generation: gen}
		}))
	case PreviewFetchMsg:
		m.PendingFetch = false
		return m, spinCmd
	case PreviewLoadedMsg:
		if msg.Repo == m.selectedRepo && msg.Number == m.selectedNumber {
			preview := msg.Preview
			m.preview = &preview
			m.Loading = false
			m.PendingFetch = false
		}
		return m, spinCmd
	}
	return m, spinCmd
}

func (m *PreviewPanelModel) View() string {
	if m.Width <= 0 || m.Height <= 0 {
		return ""
	}

	innerW := m.Width - 2
	if innerW < 1 {
		innerW = 1
	}

	oldW := m.Width
	m.Width = innerW
	defer func() { m.Width = oldW }()

	padStyle := lipgloss.NewStyle().Padding(0, 1)

	lines := m.buildLines()
	if len(lines) == 0 {
		// No PR selected — show "PREVIEW" header + underline + empty message
		if m.Loading {
			msg := m.spinner.View()
			if m.theme != nil {
				msg = m.theme.MutedTxt.Render(msg)
			}
			return padStyle.Render(renderBlock([]string{msg}, innerW, m.Height))
		}
		header := "PREVIEW"
		if m.theme != nil {
			header = m.theme.Bold.Render(m.theme.MutedTxt.Render(header))
		}
		underline := ""
		if m.theme != nil {
			underline = m.theme.Divider.Render(strings.Repeat("─", innerW))
		} else {
			underline = strings.Repeat("─", innerW)
		}
		empty := "Select a PR to preview"
		if m.theme != nil {
			empty = m.theme.MutedTxt.Render(empty)
		}
		return padStyle.Render(renderBlock([]string{header, underline, "", empty}, innerW, m.Height))
	}
	if m.Scroll < 0 {
		m.Scroll = 0
	}
	if m.Scroll > len(lines) {
		m.Scroll = len(lines)
	}
	end := m.Scroll + m.Height
	if end > len(lines) {
		end = len(lines)
	}
	visible := append([]string(nil), lines[m.Scroll:end]...)
	return padStyle.Render(renderBlock(visible, innerW, m.Height))
}

func (m *PreviewPanelModel) buildLines() []string {
	var snap domain.PRPreviewSnapshot
	switch {
	case m.preview != nil:
		snap = *m.preview
	case m.summary != nil:
		snap = makeDerivedPreview(*m.summary)
	default:
		return nil
	}

	var lines []string

	// Header = PR title as panel header (styled like other panel headers)
	var header string
	if m.theme != nil {
		header = m.theme.Header.Width(m.Width).Render(snap.Title)
	} else {
		header = fitLine(snap.Title, m.Width)
	}
	lines = append(lines, header, "")

	// Repo + Number
	var repoNum string
	if m.theme != nil {
		repoNum = m.theme.SecondaryTxt.Render(fmt.Sprintf("%s  #%d", snap.Repo, snap.Number))
	} else {
		repoNum = fmt.Sprintf("%s  #%d", snap.Repo, snap.Number)
	}
	lines = append(lines, repoNum)

	// Metadata — split across two lines so narrow panels don't lose CI/Review info
	lines = append(lines, m.authorStateLine(snap))
	lines = append(lines, m.ciReviewLine(snap))

	// Timestamps — one per line, separated from metadata by a blank line
	lines = append(lines, "", m.createdLine(snap))
	lines = append(lines, m.updatedLine(snap))

	// Loading indicator (if body/preview not yet loaded)
	if m.Loading && m.preview == nil {
		lines = append(lines, "", m.spinner.View()+" Loading preview...")
	}

	// Body
	if body := strings.TrimSpace(snap.BodyExcerpt); body != "" {
		lines = append(lines, "", m.divider(), m.sectionHeader("Body:"))
		wrapped := wrapParagraph(body, maxWidth(m.Width-2, 1))
		lines = append(lines, wrapped...)
		if !strings.HasSuffix(body, "...") {
			lines = append(lines, "...")
		}
	}

	// Top files
	if len(snap.TopFiles) > 0 {
		lines = append(lines, "", m.divider(), m.sectionHeader("Top files:"))
		for _, file := range snap.TopFiles {
			lines = append(lines, "  "+m.fileLine(file))
		}
		// "+N more files" when FileCount > len(TopFiles)
		if snap.FileCount > len(snap.TopFiles) {
			more := snap.FileCount - len(snap.TopFiles)
			if m.theme != nil {
				lines = append(lines, "  "+m.theme.MutedTxt.Render(fmt.Sprintf("+%d more files", more)))
			} else {
				lines = append(lines, fmt.Sprintf("  +%d more files", more))
			}
		}
	}

	// Latest activity
	if snap.LatestActivity != nil {
		act := snap.LatestActivity
		lines = append(lines, "", m.divider(), m.sectionHeader("Latest activity:"))
		lines = append(lines, "  "+m.activityLine(act))
		if body := strings.TrimSpace(act.Body); body != "" {
			lines = append(lines, wrapParagraph(body, maxWidth(m.Width-4, 1))...)
		}
	}

	// CI checks
	if len(snap.Checks) > 0 {
		lines = append(lines, "", m.divider(), m.sectionHeader("CI checks:"))
		limit := len(snap.Checks)
		more := 0
		if limit > 6 {
			more = limit - 6
			limit = 6
		}
		for i := 0; i < limit; i++ {
			lines = append(lines, "  "+m.checkLine(snap.Checks[i]))
		}
		if more > 0 {
			if m.theme != nil {
				lines = append(lines, "  "+m.theme.MutedTxt.Render(fmt.Sprintf("+%d more", more)))
			} else {
				lines = append(lines, fmt.Sprintf("  +%d more", more))
			}
		}
	}

	return lines
}

func (m *PreviewPanelModel) authorStateLine(snap domain.PRPreviewSnapshot) string {
	if m.theme != nil {
		return fmt.Sprintf("%s %s | %s %s",
			m.theme.MutedTxt.Render("Author:"), snap.Author,
			m.theme.MutedTxt.Render("State:"), stateLabel(snap.State, snap.IsDraft),
		)
	}
	return fmt.Sprintf("Author: %s | State: %s", snap.Author, stateLabel(snap.State, snap.IsDraft))
}

func (m *PreviewPanelModel) ciReviewLine(snap domain.PRPreviewSnapshot) string {
	if m.theme != nil {
		return fmt.Sprintf("%s %s | %s %s",
			m.theme.MutedTxt.Render("CI:"), ciLabel(snap.CIStatus),
			m.theme.MutedTxt.Render("Review:"), reviewLabel(snap.ReviewDecision, snap.IsDraft),
		)
	}
	return fmt.Sprintf("CI: %s | Review: %s", ciLabel(snap.CIStatus), reviewLabel(snap.ReviewDecision, snap.IsDraft))
}

func (m *PreviewPanelModel) createdLine(snap domain.PRPreviewSnapshot) string {
	ts := fmt.Sprintf("Created: %s", formatTimestamp(snap.CreatedAt))
	if m.theme != nil {
		return m.theme.MutedTxt.Render(ts)
	}
	return ts
}

func (m *PreviewPanelModel) updatedLine(snap domain.PRPreviewSnapshot) string {
	ts := fmt.Sprintf("Updated: %s", formatTimestamp(snap.UpdatedAt))
	if m.theme != nil {
		return m.theme.MutedTxt.Render(ts)
	}
	return ts
}

func (m *PreviewPanelModel) fileLine(file domain.PreviewFileStat) string {
	statsWidth := 12
	budget := m.Width - 2 // caller prefixes "  "
	pathBudget := budget - statsWidth
	if pathBudget < 5 {
		pathBudget = 5
	}

	path := truncatePathLeft(file.Path, pathBudget)
	path = lipgloss.NewStyle().Width(pathBudget).Align(lipgloss.Left).Render(path)

	addPart := fmt.Sprintf("+%d", file.Additions)
	delPart := fmt.Sprintf("-%d", file.Deletions)

	var statsContent string
	if m.theme != nil {
		statsContent = m.theme.Additions.Render(addPart) + " " + m.theme.Deletions.Render(delPart)
	} else {
		statsContent = addPart + " " + delPart
	}

	statsStr := lipgloss.NewStyle().Width(statsWidth).Align(lipgloss.Right).Render(statsContent)
	return path + statsStr
}

func (m *PreviewPanelModel) activityLine(act *domain.ActivitySnippet) string {
	if m.theme != nil {
		return fmt.Sprintf("%s by %s at %s", m.theme.MutedTxt.Render(activityLabel(act.Kind)), act.Author, formatTimestamp(act.OccuredAt))
	}
	return fmt.Sprintf("%s by %s at %s", activityLabel(act.Kind), act.Author, formatTimestamp(act.OccuredAt))
}

func (m *PreviewPanelModel) checkLine(check domain.PreviewCheckRow) string {
	// "  " indent (added by caller) + icon (1 char) + " " + name
	name := truncateRunes(check.Name, maxWidth(m.Width-4, 1))
	if m.theme != nil {
		return m.checkIconStyled(check.State) + " " + name
	}
	return checkIcon(check.State) + " " + name
}

func (m *PreviewPanelModel) checkIconStyled(state string) string {
	if m.theme == nil {
		return checkIcon(state)
	}
	switch state {
	case "SUCCESS":
		return m.theme.CISuccess.Render("✓")
	case "FAILURE", "ERROR":
		return m.theme.CIFailure.Render("✗")
	default:
		return m.theme.CIPending.Render("·")
	}
}

func (m *PreviewPanelModel) sectionHeader(label string) string {
	if m.theme != nil {
		return m.theme.Bold.Render(m.theme.MutedTxt.Render(label))
	}
	return label
}

func (m *PreviewPanelModel) divider() string {
	if m.theme != nil {
		return m.theme.Divider.Render(strings.Repeat("─", m.Width))
	}
	return strings.Repeat("─", m.Width)
}

func checkIcon(state string) string {
	switch state {
	case "SUCCESS":
		return "✓"
	case "FAILURE", "ERROR":
		return "✗"
	default:
		return "·"
	}
}

func (m *PreviewPanelModel) clampScroll() {
	lines := m.buildLines()
	if len(lines) == 0 {
		m.Scroll = 0
		return
	}
	if m.Scroll < 0 {
		m.Scroll = 0
	}
	maxScroll := len(lines) - m.Height
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.Scroll > maxScroll {
		m.Scroll = maxScroll
	}
}

func wrapParagraph(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{""}
	}
	var lines []string
	current := words[0]
	for _, word := range words[1:] {
		candidate := current + " " + word
		if len([]rune(candidate)) > width {
			lines = append(lines, current)
			current = word
			continue
		}
		current = candidate
	}
	lines = append(lines, current)
	return lines
}

func maxWidth(width, min int) int {
	if width < min {
		return min
	}
	return width
}

// truncateRunes truncates a plain-text (no ANSI) string to at most n runes from the left.
func truncateRunes(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}

// truncatePathLeft truncates a file path from the left so the filename (right side) stays
// visible. Uses a leading "…" to signal the path was shortened.
// e.g. "internal/foo/bar/baz.go" → "…foo/bar/baz.go" (fits in given width).
func truncatePathLeft(path string, width int) string {
	runes := []rune(path)
	if len(runes) <= width {
		return path
	}
	if width <= 1 {
		return string(runes[len(runes)-width:])
	}
	// "…" occupies 1 column; keep the last (width-1) runes of the path.
	return "…" + string(runes[len(runes)-(width-1):])
}
