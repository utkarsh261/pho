package prdetail

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/utkarsh261/pho/internal/ui/theme"
)

type composeMode int

const (
	composeModeNew           composeMode = iota // new PR-level comment
	composeModeReply                            // quote-reply to an existing entry
	composeModeApprove                          // approve the PR with an optional comment
	composeModeReviewComment                    // submit a review with COMMENT decision
	composeModeDraftInline                      // draft inline comment on selected diff lines
	composeModeEditTitle                        // edit PR title
	composeModeEditBody                         // edit PR body
)

type composeStatus int

const (
	composeStatusIdle composeStatus = iota
	composeStatusPosting
	composeStatusSuccess
	composeStatusError
)

// submitComposeMsg is emitted when the user presses Enter with non-empty input.
type submitComposeMsg struct{ body string }

// submitApproveMsg is emitted when the user presses Enter in approve mode (body may be empty).
type submitApproveMsg struct{ body string }

// openEditorComposeMsg is emitted when the user presses Ctrl+E.
type openEditorComposeMsg struct{ draft string }

// composeClosedMsg is emitted when the compose pane closes itself (e.g. via Esc).
type composeClosedMsg struct{ mode composeMode }

// ComposeModel is the bottom two-row compose pane shown when writing a comment.
type ComposeModel struct {
	active     bool
	mode       composeMode
	target     commentEntry // populated for reply mode; zero value for new comment
	input      textinput.Model
	rawBody    string // full multi-line text from $EDITOR; empty when user is typing in input
	status     composeStatus
	errMsg     string
	theme      *theme.Theme
	draftCount int // number of draft inline comments (shown in review/approve hints)
}

func newComposeModel(th *theme.Theme) ComposeModel {
	ti := textinput.New()
	ti.CharLimit = 0 // unlimited
	ti.SetCursorMode(textinput.CursorStatic)
	ti.Prompt = "" // the compose box draws its own prompt
	return ComposeModel{
		input: ti,
		theme: th,
	}
}

// Open activates the compose pane in the given mode.
func (c *ComposeModel) Open(mode composeMode, target commentEntry, draftCount int) {
	c.active = true
	c.mode = mode
	c.target = target
	c.draftCount = draftCount
	c.status = composeStatusIdle
	c.errMsg = ""
	c.input.Reset()
	c.input.Focus()
}

// Close deactivates the compose pane.
func (c *ComposeModel) Close() {
	c.active = false
	c.input.Blur()
	c.input.Reset()
	c.rawBody = ""
	c.status = composeStatusIdle
	c.errMsg = ""
}

// SetText replaces the input text (used after returning from $EDITOR).
// The full text (including newlines) is stored in rawBody for submission;
// the textinput shows only the first line as a preview.
func (c *ComposeModel) SetText(s string) {
	c.rawBody = s
	display := s
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		display = s[:idx] + "…"
	}
	c.input.SetValue(display)
	c.input.CursorEnd()
}

// Update handles key events when the compose pane is active.
// Returns the updated model, a tea.Cmd, and optionally a submitComposeMsg or
// openEditorComposeMsg (returned as the second tea.Msg return).
func (c ComposeModel) Update(msg tea.Msg) (ComposeModel, tea.Cmd) {
	if !c.active {
		return c, nil
	}

	// During posting/success, only Esc on error is handled.
	if c.status == composeStatusPosting || c.status == composeStatusSuccess {
		return c, nil
	}

	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		// Pass non-key messages to textinput.
		var cmd tea.Cmd
		c.input, cmd = c.input.Update(msg)
		return c, cmd
	}

	switch keyMsg.String() {
	case "enter":
		body := c.rawBody
		if body == "" {
			body = strings.TrimSpace(c.input.Value())
		}
		if c.mode == composeModeApprove {
			c.status = composeStatusPosting
			return c, func() tea.Msg { return submitApproveMsg{body: body} }
		}
		if c.mode == composeModeReviewComment {
			// Allow empty body when drafts exist; PRDetailModel handles the no-op.
			c.status = composeStatusPosting
			return c, func() tea.Msg { return submitComposeMsg{body: body} }
		}
		if c.mode == composeModeEditTitle || c.mode == composeModeEditBody {
			c.status = composeStatusPosting
			return c, func() tea.Msg { return submitComposeMsg{body: body} }
		}
		if body == "" {
			return c, nil // silent no-op
		}
		c.status = composeStatusPosting
		return c, func() tea.Msg { return submitComposeMsg{body: body} }

	case "ctrl+e":
		draft := c.rawBody
		if draft == "" {
			draft = c.input.Value()
		}
		return c, func() tea.Msg { return openEditorComposeMsg{draft: draft} }

	case "esc":
		// Silent discard — no confirmation regardless of content.
		wasMode := c.mode
		if c.status == composeStatusError {
			c.errMsg = ""
			c.status = composeStatusIdle
		}
		c.Close()
		return c, func() tea.Msg { return composeClosedMsg{mode: wasMode} }

	default:
		// User is editing the input directly: discard editor content so the
		// typed text (not the original editor body) is submitted.
		c.rawBody = ""
		var cmd tea.Cmd
		c.input, cmd = c.input.Update(msg)
		return c, cmd
	}
}

// View renders the compose pane at the given width as a 3-row rounded box:
// the title sits in the top border, the input in the middle, and key hints in
// the bottom border.
func (c *ComposeModel) View(width int) string {
	if !c.active {
		return ""
	}
	w := max(width-2, 1) // inner width between the side borders

	var th *theme.Theme
	if c.theme != nil {
		th = c.theme
	} else {
		th = theme.Default()
	}

	title, hint := c.titleAndHint()
	accent := th.Primary
	var body string

	switch c.status {
	case composeStatusPosting:
		body = th.MutedTxt.Render("Posting…")
		hint = ""

	case composeStatusSuccess:
		switch c.mode {
		case composeModeApprove:
			body = th.CISuccess.Render("✓ Approved")
		case composeModeReviewComment:
			body = th.CISuccess.Render("✓ Review posted")
		case composeModeEditTitle:
			body = th.CISuccess.Render("✓ Title updated")
		case composeModeEditBody:
			body = th.CISuccess.Render("✓ Body updated")
		default:
			body = th.CISuccess.Render("✓ Comment posted")
		}
		accent = th.Success
		hint = ""

	case composeStatusError:
		body = th.ReviewChanges.Render("✗ Failed: " + c.errMsg)
		accent = th.Error
		hint = "Esc: Dismiss"

	default: // idle
		prompt := lipgloss.NewStyle().Foreground(accent).Bold(true).Render("› ")
		c.input.Width = max(w-2-lipgloss.Width(prompt)-1, 10)
		c.input.Placeholder = c.placeholder()
		c.input.PlaceholderStyle = th.FaintTxt
		body = prompt + c.input.View()
	}

	border := lipgloss.NewStyle().Foreground(accent)
	top := c.borderLine(w, "╭", "╮", lipgloss.NewStyle().Foreground(th.TextBright).Bold(true).Render(title), border)
	var hintStyled string
	if hint != "" {
		hintStyled = th.RenderHints(hint)
	}
	bottom := c.borderLine(w, "╰", "╯", hintStyled, border)
	mid := border.Render("│") + " " + fitLine(body, max(w-2, 1)) + " " + border.Render("│")
	return top + "\n" + mid + "\n" + bottom
}

// borderLine renders "╭─ label ───────╮" with the label embedded near the left.
func (c *ComposeModel) borderLine(w int, left, right, label string, border lipgloss.Style) string {
	if label == "" {
		return border.Render(left + strings.Repeat("─", w) + right)
	}
	if lipgloss.Width(label) > w-4 {
		label = truncateText(label, max(w-4, 1))
	}
	fill := max(w-3-lipgloss.Width(label), 0)
	return border.Render(left+"─ ") + label + border.Render(" "+strings.Repeat("─", fill)+right)
}

// titleAndHint returns the box title and the key-hint string for the current mode.
func (c *ComposeModel) titleAndHint() (string, string) {
	drafts := ""
	if c.draftCount > 0 {
		drafts = fmt.Sprintf(" · %d draft comments", c.draftCount)
	}
	switch c.mode {
	case composeModeReply:
		if c.target.threadID != "" && c.target.path != "" && c.target.line > 0 {
			return fmt.Sprintf("Reply on %s:%d", c.target.path, c.target.line), "Enter: Send | Ctrl+E: Editor | Esc: Cancel"
		}
		if c.target.login != "" {
			return "Reply to " + c.target.login, "Enter: Send | Ctrl+E: Editor | Esc: Cancel"
		}
		return "New comment", "Enter: Send | Ctrl+E: Editor | Esc: Cancel"
	case composeModeApprove:
		return "Approve" + drafts, "Enter: Approve | Ctrl+E: Editor | Esc: Cancel"
	case composeModeReviewComment:
		return "Submit review" + drafts, "Enter: Send | Ctrl+E: Editor | Esc: Cancel"
	case composeModeDraftInline:
		return "Draft inline comment", "Enter: Save draft | Ctrl+E: Editor | Esc: Cancel"
	case composeModeEditTitle:
		return "Edit title", "Enter: Save | Ctrl+E: Editor | Esc: Cancel"
	case composeModeEditBody:
		return "Edit description", "Enter: Save | Ctrl+E: Editor | Esc: Cancel"
	default:
		return "New comment", "Enter: Send | Ctrl+E: Editor | Esc: Cancel"
	}
}

// placeholder returns the greyed-out prompt shown in an empty input.
func (c *ComposeModel) placeholder() string {
	switch c.mode {
	case composeModeApprove:
		return "Optional message…"
	case composeModeReviewComment:
		return "Summarise your review (optional)…"
	case composeModeEditTitle, composeModeEditBody:
		return ""
	default:
		return "Write a comment… (markdown supported)"
	}
}

// buildReplyBody constructs the GitHub blockquote-prefixed body for a reply.
func buildReplyBody(target commentEntry, inputText string) string {
	quotedBody := strings.TrimSpace(target.body)
	if quotedBody == "" {
		return strings.TrimSpace(inputText)
	}
	lines := strings.Split(quotedBody, "\n")
	quoted := make([]string, len(lines))
	for i, l := range lines {
		quoted[i] = "> " + l
	}
	header := "> @" + target.login + " said:"
	body := strings.TrimSpace(inputText)
	return header + "\n" + strings.Join(quoted, "\n") + "\n\n" + body
}
