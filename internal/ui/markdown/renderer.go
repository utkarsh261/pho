package markdown

import (
	"strings"
	"sync"

	"github.com/charmbracelet/glamour"
	glamouransi "github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
)

// zeroUint is a shared pointer to 0 used when zeroing style margins.
var zeroUint = func() *uint { u := uint(0); return &u }()

// tokyoNightNoMargin is the tokyo-night StyleConfig with Document and CodeBlock
// margins zeroed so output sits flush with the surrounding UI elements.
var tokyoNightNoMargin = func() glamouransi.StyleConfig {
	s := styles.TokyoNightStyleConfig
	s.Document.Margin = zeroUint
	s.CodeBlock.Margin = zeroUint
	return s
}()

// Renderer wraps a glamour TermRenderer, lazily re-creating it only when the
// requested width changes. Safe for use from a single goroutine (bubbletea's
// update/view cycle); the mutex guards the lazy re-init only.
type Renderer struct {
	mu    sync.Mutex
	width int
	inner *glamour.TermRenderer
}

// New returns a Renderer ready for use. The inner glamour renderer is
// initialized on the first Render call.
func New() *Renderer {
	return &Renderer{}
}

// Render converts markdown src into a slice of plain terminal lines ready for
// a line-based viewport. Width is the usable column count; the glamour
// renderer is re-created lazily when width differs from the previous call.
//
// Returns []string{""} for empty/whitespace-only input.
// Falls back to []string{src} if glamour fails to initialize or render.
func (r *Renderer) Render(src string, width int) []string {
	if strings.TrimSpace(src) == "" {
		return []string{""}
	}
	w := max(width, 20)

	r.mu.Lock()
	if r.inner == nil || r.width != w {
		tr, err := glamour.NewTermRenderer(
			glamour.WithStyles(tokyoNightNoMargin),
			glamour.WithWordWrap(w),
		)
		if err == nil {
			r.inner = tr
			r.width = w
		}
	}
	inner := r.inner
	r.mu.Unlock()

	if inner == nil {
		return []string{src}
	}
	out, err := inner.Render(src)
	if err != nil {
		return []string{src}
	}
	return trimOutput(strings.Split(out, "\n"))
}

// trimOutput post-processes glamour's output lines:
//   - strips trailing spaces per line (glamour right-pads to word-wrap width)
//   - drops blank lines at the head and tail (glamour wraps output in blank padding)
//   - preserves interior blank lines (paragraph spacing)
func trimOutput(lines []string) []string {
	result := make([]string, 0, len(lines))
	for _, l := range lines {
		result = append(result, strings.TrimRight(l, " "))
	}
	for len(result) > 0 && strings.TrimSpace(result[0]) == "" {
		result = result[1:]
	}
	for len(result) > 0 && strings.TrimSpace(result[len(result)-1]) == "" {
		result = result[:len(result)-1]
	}
	if len(result) == 0 {
		return []string{""}
	}
	return result
}
