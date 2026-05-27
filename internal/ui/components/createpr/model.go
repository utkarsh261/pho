package createpr

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

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
	active   bool
	repo     domain.Repository
	form     *huh.Form
	formData cmds.CreatePRFormData
	status   overlayStatus
	errMsg   string
	theme    *theme.Theme
	width    int
	height   int

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
// Returns a tea.Cmd that must be run to initialize the form.
func (m *Model) SetFormData(data cmds.CreatePRFormData) tea.Cmd {
	m.formData = data
	m.status = overlayStatusIdle

	// Build branch lists (deduped, current/default first, capped to 5).
	m.baseBranches = limitBranches(
		data.DefaultBase,
		filterOut([]string{data.DefaultBase}, allBranches(data)),
	)
	m.headBranches = limitBranches(
		data.CurrentBranch,
		filterOut([]string{data.CurrentBranch}, allBranches(data)),
	)

	// Build the form.
	m.form = m.buildForm()
	return m.form.Init()
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

	// Resolve editor: $EDITOR env var, or vim fallback.
	editorCmd := "vim"
	if ed := os.Getenv("EDITOR"); ed != "" {
		editorCmd = ed
	}

	group := huh.NewGroup(
		huh.NewSelect[string]().
			Key("base").
			Title("Base branch").
			Inline(true).
			Options(branchOptions(m.baseBranches)...).
			Value(&base),

		huh.NewSelect[string]().
			Key("head").
			Title("Head branch").
			Inline(true).
			Options(branchOptions(m.headBranches)...).
			Value(&head),

		huh.NewConfirm().
			Key("draft").
			Title("Draft PR?").
			Affirmative("Yes").
			Negative("No").
			Inline(true).
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
			Lines(4).
			Value(&body).
			Editor(editorCmd),

		&submitButton{},
	)

	form := huh.NewForm(group).WithKeyMap(km)
	// When the submit button (last field) emits NextField the form will
	// reach the last group and call SubmitCmd. We convert that into our
	// SubmitMsg so the app model can handle validation and API submission.
	form.SubmitCmd = func() tea.Msg { return SubmitMsg{} }
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
		if keyMsg.Type == tea.KeyCtrlS || keyMsg.String() == "ctrl+s" {
			return func() tea.Msg { return SubmitMsg{} }
		}
		if keyMsg.String() == "esc" {
			return func() tea.Msg { return CancelMsg{} }
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

	// Verify the selected head branch exists on the remote.
	if m.repo.LocalPath != "" {
		remoteBranch := "origin/" + head
		if out, err := execGit(m.repo.LocalPath, "branch", "-r", "--list", remoteBranch); err != nil || strings.TrimSpace(out) == "" {
			return domain.CreatePRParams{}, fmt.Errorf("branch %q has not been pushed to origin — run: git push -u origin %s", head, head)
		}
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

// View renders the overlay box (centered, with border).
func (m *Model) View() string {
	if !m.active {
		return ""
	}
	return m.renderContent(m.width, m.height, true)
}

// PanelView renders the form content sized to fit inside a dashboard panel.
// No outer border or centering — the panel already provides borders.
func (m *Model) PanelView(contentW, contentH int) string {
	if !m.active {
		return ""
	}
	return m.renderContent(contentW, contentH, false)
}

func (m *Model) renderContent(maxW, maxH int, withBorder bool) string {
	th := m.theme
	if th == nil {
		th = theme.Default()
	}

	boxW := maxW
	boxH := maxH
	if withBorder {
		// Floating overlay with rounded border.
		boxW = int(float64(maxW) * 0.7)
		boxH = int(float64(maxH) * 0.75)
		if boxW < 50 {
			boxW = 50
		}
		if boxH < 20 {
			boxH = 20
		}
		if boxW > maxW-4 {
			boxW = maxW - 4
		}
		if boxH > maxH-4 {
			boxH = maxH - 4
		}
	}

	if m.form != nil {
		formW := maxW - 2
		if withBorder {
			formW = boxW - 6
		}
		if formW < 24 {
			formW = 24
		}
		m.form.WithWidth(formW)
		m.form.WithShowHelp(withBorder)
	}

	var content string
	switch m.status {
	case overlayStatusLoading:
		content = th.MutedTxt.Render("Loading repository info…")
	case overlayStatusSubmitting:
		content = th.MutedTxt.Render("Creating pull request…")
	case overlayStatusError:
		if m.form != nil {
			content = m.form.View() + "\n" + th.ReviewChanges.Render("✗ "+m.errMsg)
		} else {
			content = th.ReviewChanges.Render("✗ " + m.errMsg)
		}
	default:
		if m.form != nil {
			content = m.form.View()
		} else {
			content = th.MutedTxt.Render("Loading…")
		}
	}

	if !withBorder {
		return m.renderPanelContent(content, maxW, th)
	}

	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(th.Border).
		Width(boxW-6).
		Padding(1, 2)

	box := borderStyle.Render(content)
	return lipgloss.Place(maxW, maxH, lipgloss.Center, lipgloss.Center, box)
}

func (m *Model) renderPanelContent(content string, width int, th *theme.Theme) string {
	if width <= 0 {
		return content
	}

	headerText := "▸ Create PR"
	header := fitWidth(headerText, width)
	if th != nil {
		header = th.Header.Width(width).Render(headerText)
	}

	lines := []string{header}
	if meta := m.panelContextLine(); meta != "" {
		if th != nil {
			meta = th.MutedTxt.Render(meta)
		}
		lines = append(lines, fitWidth(meta, width))
	}

	lines = append(lines, "")
	lines = append(lines, strings.Split(content, "\n")...)
	lines = append(lines, "")

	footer := m.footerHint()
	if th != nil {
		footer = th.MutedTxt.Render(footer)
	}
	lines = append(lines, fitWidth(footer, width))
	return strings.Join(lines, "\n")
}

func (m *Model) panelContextLine() string {
	repo := strings.TrimSpace(m.repo.FullName)
	if repo == "" {
		repo = strings.TrimSpace(m.formData.Repo.FullName)
	}
	if m.formData.IsFork && m.formData.ParentFullName != "" && repo != "" {
		repo += " → " + m.formData.ParentFullName
	}

	head := strings.TrimSpace(m.formData.CurrentBranch)
	base := strings.TrimSpace(m.formData.DefaultBase)
	if m.form != nil {
		if v := strings.TrimSpace(m.form.GetString("head")); v != "" {
			head = v
		}
		if v := strings.TrimSpace(m.form.GetString("base")); v != "" {
			base = v
		}
	}

	route := ""
	if head != "" && base != "" {
		route = head + " → " + base
	} else if head != "" {
		route = head
	} else if base != "" {
		route = "base " + base
	}

	switch {
	case repo != "" && route != "":
		return repo + "  •  " + route
	case repo != "":
		return repo
	default:
		return route
	}
}

func (m *Model) footerHint() string {
	switch m.status {
	case overlayStatusLoading, overlayStatusSubmitting:
		return "Esc: Cancel"
	default:
		return "Tab: Next field   ←/→: Change option   Ctrl+S: Create PR   Esc: Cancel"
	}
}

// ViewOver composites the overlay onto the background.
func (m *Model) ViewOver(bg string) string {
	overlay := m.View()
	if overlay == "" {
		return bg
	}

	bgLines := strings.Split(bg, "\n")
	ovlLines := strings.Split(overlay, "\n")

	boxW := min(max(int(float64(m.width)*0.7), 50), m.width-4)
	boxH := len(ovlLines)

	startRow := max((m.height-boxH)/2, 0)
	startCol := max((m.width-boxW)/2, 0)

	result := make([]string, len(bgLines))
	copy(result, bgLines)

	for i, ovlLine := range ovlLines {
		rowIdx := startRow + i
		if rowIdx < 0 || rowIdx >= len(result) {
			continue
		}
		bgLine := result[rowIdx]
		bgWidth := ansi.StringWidth(bgLine)

		// Pad background line to reach startCol if needed.
		if bgWidth < startCol {
			bgLine = bgLine + strings.Repeat(" ", startCol-bgWidth)
			bgWidth = startCol
		}

		ovlWidth := ansi.StringWidth(ovlLine)
		left := ansi.Cut(bgLine, 0, startCol)
		right := ansi.Cut(bgLine, startCol+ovlWidth, bgWidth)
		result[rowIdx] = left + ovlLine + right
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

	// Add breathing room between fields.
	ht.FieldSeparator = lipgloss.NewStyle().SetString("\n\n")

	// Make field titles bold and more prominent.
	ht.Focused.Title = lipgloss.NewStyle().Foreground(th.Primary).Bold(true)
	ht.Blurred.Title = lipgloss.NewStyle().Foreground(th.Secondary).Bold(true)
	ht.Group.Title = lipgloss.NewStyle().Foreground(th.Primary).Bold(true)
	ht.Group.Description = lipgloss.NewStyle().Foreground(th.Muted)

	ht.Focused.Description = lipgloss.NewStyle().Foreground(th.MutedTxt.GetForeground())
	ht.Blurred.Description = lipgloss.NewStyle().Foreground(th.MutedTxt.GetForeground())
	ht.Focused.TextInput.Prompt = lipgloss.NewStyle().Foreground(th.PrimaryTxt.GetForeground())
	ht.Focused.TextInput.Text = lipgloss.NewStyle().Foreground(th.PrimaryTxt.GetForeground())
	ht.Blurred.TextInput.Prompt = lipgloss.NewStyle().Foreground(th.MutedTxt.GetForeground())
	ht.Blurred.TextInput.Text = lipgloss.NewStyle().Foreground(th.PrimaryTxt.GetForeground())
	ht.Focused.SelectSelector = lipgloss.NewStyle().Foreground(th.CISuccess.GetForeground())
	ht.Focused.SelectedOption = lipgloss.NewStyle().Foreground(th.PrimaryTxt.GetForeground())
	ht.Focused.UnselectedOption = lipgloss.NewStyle().Foreground(th.MutedTxt.GetForeground())
	ht.Blurred.SelectSelector = lipgloss.NewStyle().Foreground(th.MutedTxt.GetForeground())
	ht.Blurred.SelectedOption = lipgloss.NewStyle().Foreground(th.PrimaryTxt.GetForeground())
	ht.Blurred.UnselectedOption = lipgloss.NewStyle().Foreground(th.MutedTxt.GetForeground())
	ht.Focused.FocusedButton = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Background(th.Primary).Bold(true).Padding(0, 1)
	ht.Focused.BlurredButton = lipgloss.NewStyle().Foreground(th.MutedTxt.GetForeground()).Padding(0, 1)
	ht.Blurred.FocusedButton = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Background(th.Primary).Bold(true).Padding(0, 1)
	ht.Blurred.BlurredButton = lipgloss.NewStyle().Foreground(th.MutedTxt.GetForeground()).Padding(0, 1)
	ht.Focused.ErrorIndicator = lipgloss.NewStyle().Foreground(th.ReviewChanges.GetForeground())
	ht.Focused.ErrorMessage = lipgloss.NewStyle().Foreground(th.ReviewChanges.GetForeground())
	ht.Blurred.ErrorIndicator = lipgloss.NewStyle().Foreground(th.ReviewChanges.GetForeground())
	ht.Blurred.ErrorMessage = lipgloss.NewStyle().Foreground(th.ReviewChanges.GetForeground())
	return ht
}

func branchOptions(branches []string) []huh.Option[string] {
	opts := make([]huh.Option[string], len(branches))
	for i, b := range branches {
		opts[i] = huh.NewOption(b, b)
	}
	return opts
}

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

// limitBranches returns a branch list with first at the front and at most 4
// additional items (5 total). This keeps the form compact and avoids internal
// scrolling in the Select fields.
func limitBranches(first string, rest []string) []string {
	const maxRest = 4
	if len(rest) > maxRest {
		rest = rest[:maxRest]
	}
	return append([]string{first}, rest...)
}

// execGit runs a git command in the given local path and returns trimmed stdout.
func execGit(localPath string, args ...string) (string, error) {
	cmdArgs := append([]string{"-C", localPath}, args...)
	out, err := exec.Command("git", cmdArgs...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func fitWidth(s string, width int) string {
	if width <= 0 {
		return ""
	}
	truncated := s
	if lipgloss.Width(truncated) > width {
		truncated = lipgloss.NewStyle().MaxWidth(width).Render(truncated)
	}
	return lipgloss.NewStyle().Width(width).Render(truncated)
}
