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
	compact  bool        // fields separated by one newline instead of a blank line
	fields   []huh.Field // the form's fields, measured to fit the panel
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
	m.fields = nil
	m.compact = false
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

	m.fields = []huh.Field{
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
			Title("Draft  ").
			Affirmative("Yes").
			Negative("No").
			Inline(true).
			Value(&draft),

		huh.NewInput().
			Key("title").
			Title("Title").
			Prompt("› ").
			Validate(huh.ValidateNotEmpty()).
			Value(&title),

		huh.NewText().
			Key("body").
			Title("Body").
			Description("Enter: newline · Ctrl+E: open $EDITOR").
			Placeholder("Describe the change (optional)").
			Lines(4).
			Value(&body).
			Editor(editorCmd),

		&submitButton{draft: &draft},
	}

	form := huh.NewForm(huh.NewGroup(m.fields...)).WithKeyMap(km)
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

// fitFormToPanel sizes the form to the rows left under the panel header.
// The fields are spaced out when they fit, packed without blank lines when
// they don't, and the form scrolls to the focused field if even that is too
// tall. huh fixes a form's height when it is built, so it is reset here on
// every render to follow the panel size and wrapped field content.
func (m *Model) fitFormToPanel(avail int) {
	avail = max(avail, 1)
	m.setCompact(m.formHeight(false) > avail)
	m.form.WithHeight(min(m.formHeight(m.compact), avail))
	// huh rebuilds the visible form only on Update, so nudge it to pick up
	// the new layout in this frame rather than after the next key press.
	m.form.Update(relayoutMsg{})
}

// relayoutMsg is a no-op message that makes the form rebuild its view.
type relayoutMsg struct{}

// formHeight is the height of the form's fields laid out spaced or compact.
func (m *Model) formHeight(compact bool) int {
	h := 0
	for i, f := range m.fields {
		if i > 0 && !compact {
			h++ // the blank line between fields
		}
		h += lipgloss.Height(f.View())
	}
	return h
}

// setCompact switches the form between spaced and compact field layout.
func (m *Model) setCompact(compact bool) {
	if m.form == nil || m.theme == nil || m.compact == compact {
		return
	}
	m.compact = compact
	m.form.WithTheme(m.toHuhTheme())
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

	errLine := ""
	if m.status == overlayStatusError {
		errW := maxW - 2
		if withBorder {
			errW = boxW - 10 // inside the box's border and padding
		}
		errLine = lipgloss.NewStyle().Foreground(th.Error).Width(max(errW, 10)).Render("✗ " + m.errMsg)
	}
	if m.form != nil && !withBorder && m.status != overlayStatusLoading && m.status != overlayStatusSubmitting {
		avail := maxH - len(m.panelHeader(maxW, th)) - 1
		if errLine != "" {
			avail -= lipgloss.Height(errLine) + 1
		}
		m.fitFormToPanel(avail)
	}

	var content string
	switch m.status {
	case overlayStatusLoading:
		content = lipgloss.NewStyle().Foreground(th.TextDim).Render("Loading repository info…")
	case overlayStatusSubmitting:
		content = lipgloss.NewStyle().Foreground(th.TextDim).Render("Creating pull request…")
	case overlayStatusError:
		if m.form != nil {
			content = m.form.View() + "\n\n" + errLine
		} else {
			content = errLine
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

	lines := append(m.panelHeader(width, th), "")
	for _, l := range strings.Split(content, "\n") {
		lines = append(lines, fitWidth(l, width))
	}
	return strings.Join(lines, "\n")
}

// panelHeader is the title, the route line and, when needed, the unpushed
// branch warning.
func (m *Model) panelHeader(width int, th *theme.Theme) []string {
	lines := []string{fitWidth(th.Header.Render("Create pull request"), width)}
	if route := m.routeLine(th); route != "" {
		lines = append(lines, fitWidth(route, width))
	}
	if warn := m.unpushedWarning(); warn != "" {
		for _, l := range strings.Split(lipgloss.NewStyle().Width(width).Render(warn), "\n") {
			lines = append(lines, fitWidth(lipgloss.NewStyle().Foreground(th.Warning).Render(l), width))
		}
	}
	return lines
}

// selectedBranches returns the head and base branches, as currently chosen
// in the form, falling back to the preflight defaults.
func (m *Model) selectedBranches() (head, base string) {
	head = strings.TrimSpace(m.formData.CurrentBranch)
	base = strings.TrimSpace(m.formData.DefaultBase)
	if m.form != nil {
		if v := strings.TrimSpace(m.form.GetString("head")); v != "" {
			head = v
		}
		if v := strings.TrimSpace(m.form.GetString("base")); v != "" {
			base = v
		}
	}
	return head, base
}

// routeLine shows where the PR goes: "head → base · owner/repo", with the
// branches emphasised.
func (m *Model) routeLine(th *theme.Theme) string {
	head, base := m.selectedBranches()
	dim := lipgloss.NewStyle().Foreground(th.TextDim)
	faint := lipgloss.NewStyle().Foreground(th.Faint)

	var parts []string
	switch {
	case head != "" && base != "":
		parts = append(parts, lipgloss.NewStyle().Foreground(th.AccentText).Bold(true).Render(head)+
			faint.Render(" → ")+lipgloss.NewStyle().Foreground(th.Text).Bold(true).Render(base))
	case head != "":
		parts = append(parts, lipgloss.NewStyle().Foreground(th.AccentText).Bold(true).Render(head))
	}
	if repo := m.targetRepo(); repo != "" {
		parts = append(parts, dim.Render(repo))
	}
	return strings.Join(parts, faint.Render("  ·  "))
}

// targetRepo is the repo the PR is opened against (the upstream for a fork).
func (m *Model) targetRepo() string {
	repo := strings.TrimSpace(m.repo.FullName)
	if repo == "" {
		repo = strings.TrimSpace(m.formData.Repo.FullName)
	}
	if m.formData.IsFork && m.formData.ParentFullName != "" && repo != "" {
		repo += " → " + m.formData.ParentFullName
	}
	return repo
}

// unpushedWarning warns, before the user fills in the form, when the
// selected head branch is not on origin, which Submit would reject. It sits
// under the route line, which names the branch.
func (m *Model) unpushedWarning() string {
	if m.repo.LocalPath == "" || m.status == overlayStatusLoading || m.form == nil {
		return ""
	}
	head, _ := m.selectedBranches()
	if head == "" {
		return ""
	}
	for _, b := range m.formData.RemoteBranches {
		if b == "origin/"+head {
			return ""
		}
	}
	return "▲ Not on origin yet — run git push -u origin " + head
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
	fg := func(c lipgloss.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(c) }

	// Start from huh's base theme: the focused field carries a thick left
	// bar, blurred fields a hidden one so nothing shifts as focus moves.
	ht := huh.ThemeBase()
	ht.Focused.Base = ht.Focused.Base.BorderForeground(th.Primary)
	ht.Blurred.Base = ht.Focused.Base.BorderStyle(lipgloss.HiddenBorder())
	ht.FieldSeparator = lipgloss.NewStyle().SetString("\n\n")
	if m.compact {
		ht.FieldSeparator = lipgloss.NewStyle().SetString("\n")
	}

	ht.Focused.Title = fg(th.AccentText).Bold(true)
	ht.Blurred.Title = fg(th.TextDim)
	ht.Group.Title = fg(th.Text).Bold(true)
	ht.Group.Description = fg(th.TextDim)
	ht.Focused.Description = fg(th.Faint)
	ht.Blurred.Description = fg(th.Faint)

	ht.Focused.TextInput.Prompt = fg(th.AccentText)
	ht.Blurred.TextInput.Prompt = fg(th.Faint)
	ht.Focused.TextInput.Text = fg(th.TextBright)
	ht.Blurred.TextInput.Text = fg(th.Text)
	ht.Focused.TextInput.Placeholder = fg(th.Faint)
	ht.Blurred.TextInput.Placeholder = fg(th.Faint)
	ht.Focused.TextInput.Cursor = fg(th.AccentText)

	// Inline selects: "‹ main ›", arrows only while focused.
	ht.Focused.PrevIndicator = fg(th.Faint).MarginRight(1).SetString("‹")
	ht.Focused.NextIndicator = fg(th.Faint).MarginLeft(1).SetString("›")
	ht.Blurred.PrevIndicator = lipgloss.NewStyle()
	ht.Blurred.NextIndicator = lipgloss.NewStyle()
	ht.Focused.SelectSelector = fg(th.AccentText)
	ht.Focused.SelectedOption = fg(th.TextBright).Bold(true)
	ht.Focused.UnselectedOption = fg(th.TextDim)
	ht.Blurred.SelectSelector = fg(th.Faint)
	ht.Blurred.SelectedOption = fg(th.Text)
	ht.Blurred.UnselectedOption = fg(th.TextDim)

	// Confirm (Draft) renders as a segmented Yes/No toggle; the chosen side
	// is filled. The submit button reuses FocusedButton/BlurredButton.
	seg := lipgloss.NewStyle().Padding(0, 1)
	ht.Focused.FocusedButton = seg.Foreground(th.TextBright).Background(th.Primary).Bold(true)
	ht.Focused.BlurredButton = seg.Foreground(th.TextDim).Background(th.Highlight)
	ht.Blurred.FocusedButton = seg.Foreground(th.Text).Background(th.Selection)
	ht.Blurred.BlurredButton = seg.Foreground(th.Faint).Background(th.Highlight)

	ht.Focused.ErrorIndicator = fg(th.Error).SetString(" *")
	ht.Focused.ErrorMessage = fg(th.Error)
	ht.Blurred.ErrorIndicator = fg(th.Error).SetString(" *")
	ht.Blurred.ErrorMessage = fg(th.Error)
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
