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
		if c.status == composeStatusError {
			c.errMsg = ""
			c.status = composeStatusIdle
			c.Close()
			return c, nil
		}
		c.Close()
		return c, nil

	default:
		// User is editing the input directly: discard editor content so the
		// typed text (not the original editor body) is submitted.
		c.rawBody = ""
		var cmd tea.Cmd
		c.input, cmd = c.input.Update(msg)
		return c, cmd
	}
}

// View renders the two-row compose pane at the given width.
func (c *ComposeModel) View(width int) string {
	if !c.active {
		return ""
	}
	w := max(width-2, 1)

	var th *theme.Theme
	if c.theme != nil {
		th = c.theme
	} else {
		th = theme.Default()
	}

	var row1, row2 string

	switch c.status {
	case composeStatusPosting:
		row1 = th.MutedTxt.Render("Posting…")
		row2 = ""

	case composeStatusSuccess:
		switch c.mode {
		case composeModeApprove:
			row1 = th.CISuccess.Render("✓ Approved")
		case composeModeReviewComment:
			row1 = th.CISuccess.Render("✓ Review posted")
		default:
			row1 = th.CISuccess.Render("✓ Comment posted")
		}
		row2 = ""

	case composeStatusError:
		row1 = th.ReviewChanges.Render("✗ Failed: " + c.errMsg)
		row2 = th.MutedTxt.Render("Esc: Dismiss")

	default: // idle
		var prefix string
		var hint string
		switch c.mode {
		case composeModeReply:
			if c.target.login != "" {
				prefix = "Reply to @" + c.target.login + " ▸ "
			} else {
				prefix = "New comment ▸ "
			}
			hint = "Enter: Send   Ctrl+E: $EDITOR   Esc: Cancel"
		case composeModeApprove:
			prefix = "Approve PR ▸ "
			if c.draftCount > 0 {
				hint = fmt.Sprintf("Enter: Approve   Ctrl+E: $EDITOR   Esc: Cancel   (includes +%d draft comments)", c.draftCount)
			} else {
				hint = "Enter: Approve   Ctrl+E: $EDITOR   Esc: Cancel"
			}
		case composeModeReviewComment:
			prefix = "Review comment ▸ "
			if c.draftCount > 0 {
				hint = fmt.Sprintf("Enter: Send   Ctrl+E: $EDITOR   Esc: Cancel   (includes +%d draft comments)", c.draftCount)
			} else {
				hint = "Enter: Send   Ctrl+E: $EDITOR   Esc: Cancel"
			}
		case composeModeDraftInline:
			prefix = "Draft inline comment ▸ "
			hint = "Enter: Save   Ctrl+E: $EDITOR   Esc: Cancel"
		default:
			prefix = "New comment ▸ "
			hint = "Enter: Send   Ctrl+E: $EDITOR   Esc: Cancel"
		}
		c.input.Width = max(w-lipgloss.Width(prefix)-1, 10)
		row1 = prefix + c.input.View()
		row2 = th.MutedTxt.Render(hint)
	}

	line1 := lipgloss.NewStyle().Width(w).Render(row1)
	line2 := lipgloss.NewStyle().Width(w).Render(row2)

	borderColor := th.Border
	box := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderTop(true).
		BorderBottom(false).
		BorderLeft(false).
		BorderRight(false).
		BorderForeground(borderColor).
		Width(w).
		Render(line1 + "\n" + line2)
	return box
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
