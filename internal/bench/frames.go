package bench

import (
	"slices"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// FrameBudget is one 60Hz frame.
const FrameBudget = 16 * time.Millisecond

// cmdWait bounds how long a follow-up command may run before it counts as
// background work (a tick, a debounce) that a real program would paint
// without. Waiting for it is not part of the frame.
const cmdWait = 5 * time.Millisecond

// Model is the part of tea.Model the driver needs.
type Model interface {
	Update(tea.Msg) (tea.Model, tea.Cmd)
	View() string
}

// Driver feeds messages to a model the way Bubble Tea does and records the
// work each input costs until its frame settles: every Update, every
// follow-up command that completes, and the View. Plus the bytes a
// line-diffing renderer would write for that frame.
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

// Measure records the work one input costs up to its settled frame.
func (d *Driver) Measure(msg tea.Msg) {
	work := d.settle(msg, 0)
	t0 := time.Now()
	view := d.M.View()
	d.lat = append(d.lat, work+time.Since(t0))
	lines := strings.Split(view, "\n")
	d.written = append(d.written, changedBytes(d.prev, lines))
	d.prev = lines
}

// settle applies msg and, recursively, whatever its commands produce within
// cmdWait, returning the time spent in Update and in completed commands.
func (d *Driver) settle(msg tea.Msg, depth int) time.Duration {
	if depth > 20 {
		return 0
	}
	t0 := time.Now()
	_, cmd := d.M.Update(msg)
	work := time.Since(t0)
	results := runCmds([]tea.Cmd{cmd}, time.Now().Add(cmdWait))
	for _, r := range results {
		work += r.took
	}
	for _, r := range results {
		work += d.settle(r.msg, depth+1)
	}
	return work
}

type cmdResult struct {
	msg  tea.Msg
	took time.Duration
}

// runCmds runs cmds concurrently, flattening batches, and keeps the
// messages that arrive before deadline with each command's own run time.
func runCmds(cmds []tea.Cmd, deadline time.Time) []cmdResult {
	ch := make(chan cmdResult, len(cmds))
	n := 0
	for _, c := range cmds {
		if c == nil {
			continue
		}
		n++
		go func() {
			t0 := time.Now()
			msg := c()
			ch <- cmdResult{msg, time.Since(t0)}
		}()
	}
	var out []cmdResult
	timeout := time.NewTimer(time.Until(deadline))
	defer timeout.Stop()
	for ; n > 0; n-- {
		select {
		case r := <-ch:
			switch m := r.msg.(type) {
			case nil:
			case tea.BatchMsg:
				out = append(out, runCmds(m, deadline)...)
			default:
				out = append(out, r)
			}
		case <-timeout.C:
			return out
		}
	}
	return out
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
