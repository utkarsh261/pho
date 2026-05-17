package createpr

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"

	"github.com/utkarsh261/pho/internal/application/cmds"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/ui/theme"
)

// SubmitMsg is emitted when the user presses Ctrl+S to create the PR.
type SubmitMsg struct{}

// CancelMsg is emitted when the user presses Esc to close the overlay.
type CancelMsg struct{}

// Model is the Create PR overlay.
type Model struct {
	active     bool
	repo       domain.Repository
	form       *huh.Form
	formData   cmds.CreatePRFormData
	status     overlayStatus
	errMsg     string
	theme      *theme.Theme
	width      int
	height     int

	// branch lists for dynamic OptionsFunc
	baseBranches []string
	headBranches []string
}

type overlayStatus int

const (
	overlayStatusIdle overlayStatus = iota
	overlayStatusLoading
	overlayStatusSubmitting
	overlayStatusError
)

// NewModel creates an inactive Create PR overlay.
func NewModel() *Model {
	return &Model{}
}

// Open activates the overlay with the given repository.
func (m *Model) Open(repo domain.Repository) {
	m.active = true
	m.repo = repo
	m.status = overlayStatusLoading
	m.errMsg = ""
	m.form = nil
	m.formData = cmds.CreatePRFormData{}
	m.baseBranches = nil
	m.headBranches = nil
}

// Close deactivates the overlay.
func (m *Model) Close() {
	m.active = false
	m.form = nil
}

// Active reports whether the overlay is open.
func (m *Model) Active() bool {
	return m.active
}

// SetTheme sets the color theme.
func (m *Model) SetTheme(th *theme.Theme) {
	m.theme = th
}

// SetSize sets the overlay dimensions.
func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height
}

// Init returns the preflight command.
func (m *Model) Init() tea.Cmd {
	return nil // preflight is fired by the app model
}

// SetFormData populates the overlay with preflight data and builds the form.
func (m *Model) SetFormData(data cmds.CreatePRFormData) {
	m.formData = data
	m.status = overlayStatusIdle

	// Build branch lists.
	m.baseBranches = append([]string{data.DefaultBase}, filterOut([]string{data.DefaultBase}, allBranches(data))...)
	m.headBranches = append([]string{data.CurrentBranch}, filterOut([]string{data.CurrentBranch}, allBranches(data))...)

	// Build the form.
	m.form = m.buildForm()
}

func (m *Model) buildForm() *huh.Form {
	var title, body, head, base string
	var draft bool

	title = m.formData.LastCommitMsg
	head = m.formData.CurrentBranch
	base = m.formData.DefaultBase

	km := huh.NewDefaultKeyMap()
	// Make Enter insert newlines in the body textarea.
	km.Text.Next = key.NewBinding(key.WithKeys("tab"))
	km.Text.NewLine = key.NewBinding(key.WithKeys("enter"))
	km.Text.Submit = key.NewBinding(key.WithKeys("ctrl+s"))

	// Prevent Enter from submitting select fields; use Tab instead.
	km.Select.Next = key.NewBinding(key.WithKeys("tab", "enter"))

	groups := huh.NewGroup(
		huh.NewSelect[string]().
			Key("base").
			Title("Base branch").
			Options(branchOptions(m.baseBranches)...).
			Value(&base),

		huh.NewSelect[string]().
			Key("head").
			Title("Head branch").
			Options(branchOptions(m.headBranches)...).
			Value(&head),

		huh.NewConfirm().
			Key("draft").
			Title("Draft PR?").
			Affirmative("Yes").
			Negative("No").
			Value(&draft),

		huh.NewInput().
			Key("title").
			Title("Title").
			Validate(huh.ValidateNotEmpty()).
			Value(&title),

		huh.NewText().
			Key("body").
			Title("Body").
			Description("Alt+Enter: newline  Ctrl+E: $EDITOR").
			Value(&body),
	)

	form := huh.NewForm(groups).WithKeyMap(km)
	if m.theme != nil {
		form = form.WithTheme(m.toHuhTheme())
	}
	return form
}

// Update handles messages and keys.
func (m *Model) Update(msg tea.Msg) tea.Cmd {
	if !m.active {
		return nil
	}

	if m.status == overlayStatusLoading || m.status == overlayStatusSubmitting {
		// Only Esc cancels during loading/submitting.
		if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.String() == "esc" {
			return func() tea.Msg { return CancelMsg{} }
		}
		return nil
	}

	// Custom submit key.
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if keyMsg.String() == "ctrl+s" {
			return func() tea.Msg { return SubmitMsg{} }
		}
		if keyMsg.String() == "esc" {
			return func() tea.Msg { return CancelMsg{} }
		}

		// Prevent auto-completion: swallow Tab/Enter when on the last field (body).
		if m.form != nil {
			focused := m.form.GetFocusedField()
			if focused != nil && focused.GetKey() == "body" {
				switch keyMsg.String() {
				case "tab", "enter":
					return nil
				}
			}
		}
	}

	if m.form != nil {
		newForm, cmd := m.form.Update(msg)
		if f, ok := newForm.(*huh.Form); ok {
			m.form = f
		}
		return cmd
	}

	return nil
}

// Submit extracts form values and returns a CreatePRParams.
func (m *Model) Submit() (domain.CreatePRParams, error) {
	if m.form == nil {
		return domain.CreatePRParams{}, fmt.Errorf("form not initialized")
	}

	title := m.form.GetString("title")
	body := m.form.GetString("body")
	head := m.form.GetString("head")
	base := m.form.GetString("base")
	draft := m.form.GetBool("draft")

	if strings.TrimSpace(title) == "" {
		return domain.CreatePRParams{}, fmt.Errorf("title is required")
	}

	params := domain.CreatePRParams{
		Repo:  m.repo,
		Title: strings.TrimSpace(title),
		Body:  body,
		Head:  head,
		Base:  base,
		Draft: draft,
	}

	// If this is a fork, prefix head with the fork owner.
	if m.formData.IsFork && m.formData.ParentFullName != "" {
		parts := strings.Split(m.repo.FullName, "/")
		if len(parts) == 2 {
			params.Head = parts[0] + ":" + head
		}
		// Base repo becomes the upstream.
		upParts := strings.Split(m.formData.ParentFullName, "/")
		if len(upParts) == 2 {
			params.Repo = domain.Repository{
				Host:     m.repo.Host,
				Owner:    upParts[0],
				Name:     upParts[1],
				FullName: m.formData.ParentFullName,
			}
		}
	}

	return params, nil
}

// SetError sets the overlay into error state.
func (m *Model) SetError(err error) {
	m.status = overlayStatusError
	if err != nil {
		m.errMsg = err.Error()
	}
}

// SetSubmitting sets the overlay into submitting state.
func (m *Model) SetSubmitting() {
	m.status = overlayStatusSubmitting
}

// View renders the overlay box.
func (m *Model) View() string {
	if !m.active {
		return ""
	}

	th := m.theme
	if th == nil {
		th = theme.Default()
	}

	boxW := int(float64(m.width) * 0.7)
	boxH := int(float64(m.height) * 0.75)
	if boxW < 50 {
		boxW = 50
	}
	if boxH < 20 {
		boxH = 20
	}
	if boxW > m.width-4 {
		boxW = m.width - 4
	}
	if boxH > m.height-4 {
		boxH = m.height - 4
	}

	var content string
	switch m.status {
	case overlayStatusLoading:
		content = th.MutedTxt.Render("Loading repository info…")
	case overlayStatusSubmitting:
		content = th.MutedTxt.Render("Creating pull request…")
	case overlayStatusError:
		content = m.form.View() + "\n" + th.ReviewChanges.Render("✗ "+m.errMsg)
	default:
		if m.form != nil {
			content = m.form.View()
		} else {
			content = th.MutedTxt.Render("Loading…")
		}
	}

	// Add footer hint.
	if m.status == overlayStatusIdle || m.status == overlayStatusError {
		content += "\n" + th.MutedTxt.Render("Ctrl+S: Create PR   Esc: Cancel")
	}

	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(th.Border).
		Width(boxW - 2).
		Height(boxH - 2).
		Padding(1, 2)

	box := borderStyle.Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

// ViewOver composites the overlay onto the background.
func (m *Model) ViewOver(bg string) string {
	overlay := m.View()
	if overlay == "" {
		return bg
	}

	bgLines := strings.Split(bg, "\n")
	ovlLines := strings.Split(overlay, "\n")

	boxW := int(float64(m.width) * 0.7)
	if boxW < 50 {
		boxW = 50
	}
	if boxW > m.width-4 {
		boxW = m.width - 4
	}
	boxH := len(ovlLines)

	startRow := (m.height - boxH) / 2
	if startRow < 0 {
		startRow = 0
	}
	startCol := (m.width - boxW) / 2
	if startCol < 0 {
		startCol = 0
	}

	result := make([]string, len(bgLines))
	copy(result, bgLines)

	for i, ovlLine := range ovlLines {
		rowIdx := startRow + i
		if rowIdx < 0 || rowIdx >= len(result) {
			continue
		}
		bgLine := result[rowIdx]
		if startCol+len(ovlLine) > len(bgLine) {
			// Pad bgLine if necessary.
			if startCol < len(bgLine) {
				result[rowIdx] = bgLine[:startCol] + ovlLine
			} else {
				result[rowIdx] = bgLine + strings.Repeat(" ", startCol-len(bgLine)) + ovlLine
			}
		} else {
			result[rowIdx] = bgLine[:startCol] + ovlLine + bgLine[startCol+len(ovlLine):]
		}
	}

	return strings.Join(result, "\n")
}

func (m *Model) toHuhTheme() *huh.Theme {
	th := m.theme
	if th == nil {
		th = theme.Default()
	}
	// Start from huh's base theme and override colors.
	ht := huh.ThemeBase()
	ht.Focused.Base = ht.Focused.Base.BorderForeground(th.Border)
	ht.Focused.Title = lipgloss.NewStyle().Foreground(th.BoxTitle.GetForeground())
	ht.Focused.Description = lipgloss.NewStyle().Foreground(th.MutedTxt.GetForeground())
	ht.Focused.TextInput.Prompt = lipgloss.NewStyle().Foreground(th.PrimaryTxt.GetForeground())
	ht.Focused.TextInput.Text = lipgloss.NewStyle().Foreground(th.PrimaryTxt.GetForeground())
	ht.Focused.SelectSelector = lipgloss.NewStyle().Foreground(th.CISuccess.GetForeground())
	ht.Focused.SelectedOption = lipgloss.NewStyle().Foreground(th.PrimaryTxt.GetForeground())
	ht.Focused.UnselectedOption = lipgloss.NewStyle().Foreground(th.MutedTxt.GetForeground())
	ht.Focused.FocusedButton = lipgloss.NewStyle().Foreground(th.PrimaryTxt.GetForeground()).Background(th.Border)
	ht.Focused.BlurredButton = lipgloss.NewStyle().Foreground(th.MutedTxt.GetForeground())
	ht.Focused.ErrorIndicator = lipgloss.NewStyle().Foreground(th.ReviewChanges.GetForeground())
	ht.Focused.ErrorMessage = lipgloss.NewStyle().Foreground(th.ReviewChanges.GetForeground())
	return ht
}

// branchOptions converts strings to huh options.
func branchOptions(branches []string) []huh.Option[string] {
	opts := make([]huh.Option[string], len(branches))
	for i, b := range branches {
		opts[i] = huh.NewOption(b, b)
	}
	return opts
}

// allBranches returns a deduplicated list of all branches.
func allBranches(data cmds.CreatePRFormData) []string {
	seen := make(map[string]bool)
	var out []string
	for _, b := range data.LocalBranches {
		if !seen[b] {
			seen[b] = true
			out = append(out, b)
		}
	}
	for _, b := range data.RemoteBranches {
		// Strip "origin/" prefix for remote branches.
		clean := strings.TrimPrefix(b, "origin/")
		if !seen[clean] {
			seen[clean] = true
			out = append(out, clean)
		}
	}
	return out
}

// filterOut removes excluded items from src.
func filterOut(exclude []string, src []string) []string {
	set := make(map[string]bool, len(exclude))
	for _, e := range exclude {
		set[e] = true
	}
	var out []string
	for _, s := range src {
		if !set[s] {
			out = append(out, s)
		}
	}
	return out
}
