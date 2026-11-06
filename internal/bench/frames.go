package bench

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

// FrameBudget is one 60Hz frame.
const FrameBudget = 16 * time.Millisecond

// cmdWait bounds how long a follow-up command may take before it counts as
// background work (a tick, a debounce) and is dropped, as a real program
// would paint without it.
const cmdWait = 5 * time.Millisecond

// Model is the part of tea.Model the driver needs.
type Model interface {
	Update(tea.Msg) (tea.Model, tea.Cmd)
	View() string
}

// Driver feeds messages to a model the way Bubble Tea does and records how
// long each input takes to reach a settled frame, plus the bytes a
// line-diffing renderer would write for it.
type Driver struct {
	M       Model
	lat     []time.Duration
	written []int
	prev    []string
}

func NewDriver(m Model) *Driver {
	d := &Driver{M: m}
	d.prev = strings.Split(m.View(), "\n")
	return d
}

// Send delivers msg untimed and settles its follow-ups.
func (d *Driver) Send(msg tea.Msg) {
	d.settle(msg, 0)
	d.prev = strings.Split(d.M.View(), "\n")
}

// Press times each key from Update to the settled View.
func (d *Driver) Press(keys ...string) {
	for _, k := range keys {
		for _, msg := range KeyMsgs(k) {
			d.Measure(msg)
		}
	}
}

// Measure times one input to its settled frame.
func (d *Driver) Measure(msg tea.Msg) {
	start := time.Now()
	d.settle(msg, 0)
	view := d.M.View()
	d.lat = append(d.lat, time.Since(start))
	lines := strings.Split(view, "\n")
	d.written = append(d.written, changedBytes(d.prev, lines))
	d.prev = lines
}

func (d *Driver) settle(msg tea.Msg, depth int) {
	if depth > 20 {
		return
	}
	_, cmd := d.M.Update(msg)
	for _, next := range runCmd(cmd) {
		d.settle(next, depth+1)
	}
}

func runCmd(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	ch := make(chan tea.Msg, 1)
	go func() { ch <- cmd() }()
	var msg tea.Msg
	select {
	case msg = <-ch:
	case <-time.After(cmdWait):
		return nil
	}
	switch m := msg.(type) {
	case nil, spinner.TickMsg:
		return nil
	case tea.BatchMsg:
		var out []tea.Msg
		for _, c := range m {
			out = append(out, runCmd(c)...)
		}
		return out
	}
	return []tea.Msg{msg}
}

// changedBytes approximates the standard renderer, which rewrites only the
// lines that differ from the last frame.
func changedBytes(prev, next []string) int {
	n := 0
	for i, l := range next {
		if i >= len(prev) || prev[i] != l {
			n += len(l) + 1
		}
	}
	return n
}

// Reset drops samples, e.g. after warm-up keys.
func (d *Driver) Reset() { d.lat, d.written = d.lat[:0], d.written[:0] }

// Report publishes latency percentiles over every measured input, slow
// frames per op and bytes per frame.
func (d *Driver) Report(b *testing.B) {
	b.Helper()
	if len(d.lat) == 0 {
		return
	}
	s := slices.Clone(d.lat)
	slices.Sort(s)
	us := func(q float64) float64 {
		return float64(s[min(len(s)-1, int(q*float64(len(s))))].Nanoseconds()) / 1e3
	}
	slow, bytes := 0, 0
	for i, l := range d.lat {
		if l > FrameBudget {
			slow++
		}
		bytes += d.written[i]
	}
	b.ReportMetric(us(0.50), "p50-µs")
	b.ReportMetric(us(0.95), "p95-µs")
	b.ReportMetric(us(0.99), "p99-µs")
	b.ReportMetric(float64(s[len(s)-1].Nanoseconds())/1e3, "max-µs")
	b.ReportMetric(float64(slow)/float64(b.N), "slowframes/op")
	b.ReportMetric(float64(bytes)/float64(len(d.lat)), "B/frame")
}

// Latencies returns the recorded samples in input order.
func (d *Driver) Latencies() []time.Duration { return d.lat }

var namedKeys = map[string]tea.KeyType{
	"enter": tea.KeyEnter, "esc": tea.KeyEsc, "tab": tea.KeyTab, "shift+tab": tea.KeyShiftTab,
	"up": tea.KeyUp, "down": tea.KeyDown, "left": tea.KeyLeft, "right": tea.KeyRight,
	"backspace": tea.KeyBackspace, "space": tea.KeySpace,
	"ctrl+d": tea.KeyCtrlD, "ctrl+u": tea.KeyCtrlU, "ctrl+p": tea.KeyCtrlP,
	"ctrl+n": tea.KeyCtrlN, "ctrl+f": tea.KeyCtrlF, "ctrl+b": tea.KeyCtrlB,
}

// KeyMsgs turns a key name ("ctrl+d", "enter") into one press and any other
// string into one press per rune.
func KeyMsgs(k string) []tea.Msg {
	if t, ok := namedKeys[k]; ok {
		return []tea.Msg{tea.KeyMsg{Type: t}}
	}
	out := make([]tea.Msg, 0, len(k))
	for _, r := range k {
		out = append(out, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return out
}

// Repeat returns k n times, for scripts like Repeat("j", 500).
func Repeat(k string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = k
	}
	return out
}
