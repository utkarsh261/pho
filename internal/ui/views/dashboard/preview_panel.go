package dashboard

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/utk/git-term/internal/domain"
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
}

func NewPreviewPanelModel() *PreviewPanelModel {
	return &PreviewPanelModel{
		DebounceDelay: 100 * time.Millisecond,
	}
}

func (m *PreviewPanelModel) Init() tea.Cmd { return nil }

func (m *PreviewPanelModel) SetRect(width, height int) {
	m.Width = width
	m.Height = height
	m.clampScroll()
}

func (m *PreviewPanelModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.SetRect(msg.Width, msg.Height)
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			m.Scroll++
			m.clampScroll()
		case "k", "up":
			m.Scroll--
			m.clampScroll()
		}
		return m, nil
	case SelectPRMsg:
		m.selectedRepo = msg.Repo
		m.selectedNumber = msg.Number
		summary := msg.Summary
		m.summary = &summary
		m.preview = nil
		m.Loading = true
		m.Scroll = 0
		m.DebounceGeneration++
		if m.PendingFetch {
			return m, nil
		}
		m.PendingFetch = true
		gen := m.DebounceGeneration
		return m, tea.Tick(m.DebounceDelay, func(time.Time) tea.Msg {
			return PreviewFetchMsg{Repo: m.selectedRepo, Number: m.selectedNumber, Generation: gen}
		})
	case PreviewFetchMsg:
		m.PendingFetch = false
		return m, nil
	case PreviewLoadedMsg:
		if msg.Repo == m.selectedRepo && msg.Number == m.selectedNumber {
			preview := msg.Preview
			m.preview = &preview
			m.Loading = false
			m.PendingFetch = false
		}
		return m, nil
	}
	return m, nil
}

func (m *PreviewPanelModel) View() string {
	if m.Width <= 0 || m.Height <= 0 {
		return ""
	}
	lines := m.buildLines()
	if len(lines) == 0 {
		if m.Loading {
			return renderBlock([]string{"Loading preview..."}, m.Width, m.Height)
		}
		return renderBlock([]string{"No preview"}, m.Width, m.Height)
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
	return renderBlock(visible, m.Width, m.Height)
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

	lines := []string{
		snap.Title,
		fmt.Sprintf("%s  #%d", snap.Repo, snap.Number),
		fmt.Sprintf("Author: %s | State: %s | CI: %s | Review: %s", snap.Author, stateLabel(snap.State, snap.IsDraft), ciLabel(snap.CIStatus), reviewLabel(snap.ReviewDecision, snap.IsDraft)),
		fmt.Sprintf("Created: %s | Updated: %s", formatTimestamp(snap.CreatedAt), formatTimestamp(snap.UpdatedAt)),
	}

	if m.Loading && m.preview == nil {
		lines = append(lines, "Loading preview...")
	}

	if body := strings.TrimSpace(snap.BodyExcerpt); body != "" {
		lines = append(lines, "Body:")
		lines = append(lines, wrapParagraph(body, maxWidth(m.Width-2, 1))...)
		if !strings.HasSuffix(body, "...") {
			lines = append(lines, "...")
		}
	}

	if len(snap.TopFiles) > 0 {
		lines = append(lines, "Top files:")
		for _, file := range snap.TopFiles {
			lines = append(lines, fmt.Sprintf("  %+d/-%d %s", file.Additions, file.Deletions, file.Path))
		}
	}

	if snap.LatestActivity != nil {
		act := snap.LatestActivity
		lines = append(lines, "Latest activity:")
		lines = append(lines, fmt.Sprintf("  %s by %s at %s", activityLabel(act.Kind), act.Author, formatTimestamp(act.OccuredAt)))
		if body := strings.TrimSpace(act.Body); body != "" {
			lines = append(lines, wrapParagraph(body, maxWidth(m.Width-4, 1))...)
		}
	}

	if len(snap.Checks) > 0 {
		lines = append(lines, "CI checks:")
		limit := len(snap.Checks)
		more := 0
		if limit > 6 {
			more = limit - 6
			limit = 6
		}
		for i := 0; i < limit; i++ {
			check := snap.Checks[i]
			lines = append(lines, fmt.Sprintf("  %s %s", check.State, check.Name))
		}
		if more > 0 {
			lines = append(lines, fmt.Sprintf("  +%d more", more))
		}
	}

	return lines
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
