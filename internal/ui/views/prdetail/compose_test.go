package prdetail

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/ui/theme"
)

// ── buildReplyBody ────────────────────────────────────────────────────────────

func TestBuildReplyBodyQuotesLoginAndBody(t *testing.T) {
	t.Parallel()
	target := commentEntry{login: "alice", body: "Should we add error handling?"}
	got := buildReplyBody(target, "Yes, good point.")
	if !strings.Contains(got, "> @alice said:") {
		t.Errorf("expected '>  @alice said:' header in reply body, got:\n%s", got)
	}
	if !strings.Contains(got, "> Should we add error handling?") {
		t.Errorf("expected quoted body in reply, got:\n%s", got)
	}
	if !strings.HasSuffix(strings.TrimSpace(got), "Yes, good point.") {
		t.Errorf("expected user text at end, got:\n%s", got)
	}
}

func TestBuildReplyBodyEmptyTargetBody(t *testing.T) {
	t.Parallel()
	target := commentEntry{login: "bob", body: ""}
	got := buildReplyBody(target, "My reply.")
	if got != "My reply." {
		t.Errorf("expected bare reply when target body empty, got %q", got)
	}
}

func TestBuildReplyBodyMultilineQuoting(t *testing.T) {
	t.Parallel()
	target := commentEntry{login: "carol", body: "line one\nline two"}
	got := buildReplyBody(target, "ack")
	if !strings.Contains(got, "> line one") {
		t.Errorf("expected '>  line one' quoted, got:\n%s", got)
	}
	if !strings.Contains(got, "> line two") {
		t.Errorf("expected '>  line two' quoted, got:\n%s", got)
	}
}

// ── ComposeModel.Update ───────────────────────────────────────────────────────

func newActiveCompose(th *theme.Theme) ComposeModel {
	c := newComposeModel(th)
	c.Open(composeModeNew, commentEntry{}, 0)
	return c
}

func sendKey(c ComposeModel, key string) (ComposeModel, tea.Cmd) {
	var t tea.KeyType
	switch key {
	case "enter":
		t = tea.KeyEnter
	case "esc":
		t = tea.KeyEsc
	case "ctrl+e":
		t = tea.KeyCtrlE
	default:
		return c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
	}
	return c.Update(tea.KeyMsg{Type: t})
}

func TestComposeEnterEmptyIsNoop(t *testing.T) {
	t.Parallel()
	c := newActiveCompose(nil)
	c, cmd := sendKey(c, "enter")
	if cmd != nil {
		t.Error("expected nil cmd for empty enter")
	}
	if c.status != composeStatusIdle {
		t.Errorf("expected idle status, got %v", c.status)
	}
}

func TestComposeEnterWithBodyEmitsSubmit(t *testing.T) {
	t.Parallel()
	c := newActiveCompose(nil)
	c, _ = sendKey(c, "h")
	c, _ = sendKey(c, "i")
	c, cmd := sendKey(c, "enter")
	if cmd == nil {
		t.Fatal("expected non-nil cmd after enter with text")
	}
	msg := cmd()
	sm, ok := msg.(submitComposeMsg)
	if !ok {
		t.Fatalf("expected submitComposeMsg, got %T", msg)
	}
	if sm.body != "hi" {
		t.Errorf("expected body 'hi', got %q", sm.body)
	}
	if c.status != composeStatusPosting {
		t.Errorf("expected composeStatusPosting after enter, got %v", c.status)
	}
}

func TestComposeEscCloses(t *testing.T) {
	t.Parallel()
	c := newActiveCompose(nil)
	c, _ = sendKey(c, "esc")
	if c.active {
		t.Error("expected compose to be closed after esc")
	}
}

func TestComposeEscWithContentSilentlyDiscards(t *testing.T) {
	t.Parallel()
	c := newActiveCompose(nil)
	c, _ = sendKey(c, "h")
	c, _ = sendKey(c, "esc")
	if c.active {
		t.Error("expected compose closed after esc even with content")
	}
}

func TestComposeCtrlEEmitsOpenEditor(t *testing.T) {
	t.Parallel()
	c := newActiveCompose(nil)
	c, _ = sendKey(c, "h")
	c, cmd := sendKey(c, "ctrl+e")
	if cmd == nil {
		t.Fatal("expected non-nil cmd for ctrl+e")
	}
	msg := cmd()
	oe, ok := msg.(openEditorComposeMsg)
	if !ok {
		t.Fatalf("expected openEditorComposeMsg, got %T", msg)
	}
	if oe.draft != "h" {
		t.Errorf("expected draft='h', got %q", oe.draft)
	}
}

func TestComposeInactiveIgnoresKeys(t *testing.T) {
	t.Parallel()
	c := newComposeModel(nil)
	c, cmd := sendKey(c, "enter")
	if cmd != nil || c.active {
		t.Error("expected no-op when compose is inactive")
	}
}

func TestComposePostingBlocksKeys(t *testing.T) {
	t.Parallel()
	c := newActiveCompose(nil)
	c.status = composeStatusPosting
	c, cmd := sendKey(c, "esc")
	if cmd != nil {
		t.Error("expected no cmd during posting state")
	}
	if !c.active {
		t.Error("expected compose to stay open during posting")
	}
}

// ── ComposeModel.View ─────────────────────────────────────────────────────────

var composeGoldenWidths = []int{79, 80, 120}

func TestComposeViewIdleGolden(t *testing.T) {
	th := theme.Default()
	for _, w := range composeGoldenWidths {
		w := w
		t.Run(fmt.Sprintf("w%d", w), func(t *testing.T) {
			t.Parallel()
			c := newComposeModel(th)
			c.Open(composeModeNew, commentEntry{}, 0)
			got := descStripANSI(c.View(w))
			checkGolden(t, got, fmt.Sprintf("compose_idle_w%d.txt", w))
		})
	}
}

func TestComposeViewReplyGolden(t *testing.T) {
	th := theme.Default()
	for _, w := range composeGoldenWidths {
		w := w
		t.Run(fmt.Sprintf("w%d", w), func(t *testing.T) {
			t.Parallel()
			c := newComposeModel(th)
			c.Open(composeModeReply, commentEntry{login: "alice"}, 0)
			got := descStripANSI(c.View(w))
			checkGolden(t, got, fmt.Sprintf("compose_reply_w%d.txt", w))
		})
	}
}

func TestComposeViewPostingGolden(t *testing.T) {
	th := theme.Default()
	for _, w := range composeGoldenWidths {
		w := w
		t.Run(fmt.Sprintf("w%d", w), func(t *testing.T) {
			t.Parallel()
			c := newComposeModel(th)
			c.Open(composeModeNew, commentEntry{}, 0)
			c.status = composeStatusPosting
			got := descStripANSI(c.View(w))
			checkGolden(t, got, fmt.Sprintf("compose_posting_w%d.txt", w))
		})
	}
}

func TestComposeViewSuccessGolden(t *testing.T) {
	th := theme.Default()
	for _, w := range composeGoldenWidths {
		w := w
		t.Run(fmt.Sprintf("w%d", w), func(t *testing.T) {
			t.Parallel()
			c := newComposeModel(th)
			c.Open(composeModeNew, commentEntry{}, 0)
			c.status = composeStatusSuccess
			got := descStripANSI(c.View(w))
			checkGolden(t, got, fmt.Sprintf("compose_success_w%d.txt", w))
		})
	}
}

func TestComposeViewErrorGolden(t *testing.T) {
	th := theme.Default()
	for _, w := range composeGoldenWidths {
		w := w
		t.Run(fmt.Sprintf("w%d", w), func(t *testing.T) {
			t.Parallel()
			c := newComposeModel(th)
			c.Open(composeModeNew, commentEntry{}, 0)
			c.status = composeStatusError
			c.errMsg = "403 Forbidden"
			got := descStripANSI(c.View(w))
			checkGolden(t, got, fmt.Sprintf("compose_error_w%d.txt", w))
		})
	}
}

// ── comments section golden ───────────────────────────────────────────────────

func TestCommentsLinesGolden(t *testing.T) {
	for _, w := range composeGoldenWidths {
		w := w
		t.Run(fmt.Sprintf("w%d", w), func(t *testing.T) {
			t.Parallel()
			m := makePRDetail(w, 40, nil, nil)
			m.Detail = makeDetailWithGoldenComments()
			m.SetTheme(theme.Default())
			cw := m.contentW()
			lines := m.commentLines(cw, -1)
			got := descStripANSI(strings.Join(lines, "\n"))
			checkGolden(t, got, fmt.Sprintf("comments_w%d.txt", w))
		})
	}
}

func TestCommentsLinesActiveGolden(t *testing.T) {
	for _, w := range composeGoldenWidths {
		w := w
		t.Run(fmt.Sprintf("w%d", w), func(t *testing.T) {
			t.Parallel()
			m := makePRDetail(w, 40, nil, nil)
			m.Detail = makeDetailWithGoldenComments()
			m.SetTheme(theme.Default())
			cw := m.contentW()
			lines := m.commentLines(cw, 1) // second entry active
			got := descStripANSI(strings.Join(lines, "\n"))
			checkGolden(t, got, fmt.Sprintf("comments_active_w%d.txt", w))
		})
	}
}

func makeDetailWithGoldenComments() *domain.PRPreviewSnapshot {
	t1 := time.Date(2024, 3, 10, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 3, 11, 0, 0, 0, 0, time.UTC)
	return &domain.PRPreviewSnapshot{
		Reviewers: []domain.PreviewReviewer{
			{Login: "alice", State: "APPROVED", Body: "LGTM! Nice cleanup.", SubmittedAt: t1},
		},
		Comments: []domain.PreviewComment{
			{Login: "bob", Body: "Should we add error handling for nil tokens?", CreatedAt: t2},
		},
	}
}

// checkGolden compares got against testdata/golden/<name>, writing the file when -update is set.
func checkGolden(t *testing.T, got, name string) {
	t.Helper()
	goldenPath := filepath.Join("testdata", "golden", name)
	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0755); err != nil {
			t.Fatalf("mkdir golden dir: %v", err)
		}
		if err := os.WriteFile(goldenPath, []byte(got), 0644); err != nil {
			t.Fatalf("write golden %s: %v", goldenPath, err)
		}
		return
	}
	data, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden %s: %v (run with -update to generate)", goldenPath, err)
	}
	if got != string(data) {
		t.Errorf("golden mismatch for %s\ngot:\n%s\nwant:\n%s", name, got, string(data))
	}
}

