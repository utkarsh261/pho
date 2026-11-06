// Command report turns `go test -bench` output into the markdown summary the
// bench workflow posts on PRs. Given only -head it describes one run.
//
//	go run ./scripts/bench/report -base base.txt -head head.txt > report.md
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// run holds every sample per benchmark and unit, plus health-check lines.
type run struct {
	samples map[string]map[string][]float64
	order   []string
	health  map[string]string // SCALING/HEAP/SOAK name -> line
}

var (
	benchLine  = regexp.MustCompile(`^(Benchmark\S+?)(?:-\d+)?\s+\d+\s+(.*)$`)
	healthLine = regexp.MustCompile(`\b(SCALING|HEAP|SOAK) (\S+) (.*)$`)
)

func parse(path string) (*run, error) {
	r := &run{samples: map[string]map[string][]float64{}, health: map[string]string{}}
	if path == "" {
		return r, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if m := healthLine.FindStringSubmatch(line); m != nil {
			r.health[m[1]+" "+m[2]] = m[3]
			continue
		}
		m := benchLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		name := strings.TrimPrefix(m[1], "Benchmark")
		fields := strings.Fields(m[2])
		if r.samples[name] == nil {
			r.samples[name] = map[string][]float64{}
			r.order = append(r.order, name)
		}
		for i := 0; i+1 < len(fields); i += 2 {
			v, err := strconv.ParseFloat(fields[i], 64)
			if err != nil {
				continue
			}
			r.samples[name][fields[i+1]] = append(r.samples[name][fields[i+1]], v)
		}
	}
	return r, sc.Err()
}

func median(xs []float64) float64 {
	if len(xs) == 0 {
		return math.NaN()
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	if len(s)%2 == 1 {
		return s[len(s)/2]
	}
	return (s[len(s)/2-1] + s[len(s)/2]) / 2
}

// mannWhitney returns the two-sided p-value of the U test (normal
// approximation with tie correction), as benchstat uses.
func mannWhitney(a, b []float64) float64 {
	n1, n2 := float64(len(a)), float64(len(b))
	if n1 < 2 || n2 < 2 {
		return 1
	}
	type obs struct {
		v float64
		g int
	}
	all := make([]obs, 0, len(a)+len(b))
	for _, v := range a {
		all = append(all, obs{v, 0})
	}
	for _, v := range b {
		all = append(all, obs{v, 1})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].v < all[j].v })
	var r1, ties float64
	for i := 0; i < len(all); {
		j := i
		for j < len(all) && all[j].v == all[i].v {
			j++
		}
		rank := float64(i+j+1) / 2
		t := float64(j - i)
		ties += t*t*t - t
		for k := i; k < j; k++ {
			if all[k].g == 0 {
				r1 += rank
			}
		}
		i = j
	}
	u := r1 - n1*(n1+1)/2
	n := n1 + n2
	sd := math.Sqrt(n1 * n2 / 12 * ((n + 1) - ties/(n*(n-1))))
	if sd == 0 {
		return 1
	}
	z := (math.Abs(u-n1*n2/2) - 0.5) / sd
	return math.Erfc(z / math.Sqrt2)
}

const alpha = 0.05

type cmp struct {
	base, head float64
	p          float64
	have       bool // base has samples
}

func (c cmp) delta() float64 { return (c.head - c.base) / c.base }

func compare(base, head *run, name, unit string) cmp {
	h := head.samples[name][unit]
	b := base.samples[name][unit]
	c := cmp{head: median(h), base: median(b), p: 1, have: len(b) > 0}
	if c.have {
		c.p = mannWhitney(b, h)
	}
	return c
}

// verdict: higher is worse for every unit in this report.
func (c cmp) verdict() string {
	switch {
	case !c.have:
		return "new"
	case c.base == c.head:
		return "~"
	case c.p >= alpha:
		return fmt.Sprintf("~ (p=%.2f)", c.p)
	case c.head > c.base:
		return fmt.Sprintf("🔺 %+.0f%%", 100*c.delta())
	default:
		return fmt.Sprintf("✅ %+.0f%%", 100*c.delta())
	}
}

// exactVerdict is for deterministic counts, where any change is real.
func (c cmp) exactVerdict() string {
	switch {
	case !c.have:
		return "new"
	case c.head == c.base:
		return ""
	case c.base == 0:
		return fmt.Sprintf("🔺 +%s", num(c.head))
	case c.head > c.base:
		return fmt.Sprintf("🔺 %+.0f%%", 100*c.delta())
	default:
		return fmt.Sprintf("✅ %+.0f%%", 100*c.delta())
	}
}

func num(v float64) string {
	switch {
	case math.IsNaN(v):
		return "–"
	case v >= 1e6:
		return fmt.Sprintf("%.1fM", v/1e6)
	case v >= 1e4:
		return fmt.Sprintf("%.0fk", v/1e3)
	case v >= 100 || v == math.Trunc(v):
		return fmt.Sprintf("%.0f", v)
	default:
		return fmt.Sprintf("%.2f", v)
	}
}

func ms(us float64) string {
	if math.IsNaN(us) {
		return "–"
	}
	if us >= 1e6 {
		return fmt.Sprintf("%.2fs", us/1e6)
	}
	if us >= 1e4 {
		return fmt.Sprintf("%.0fms", us/1e3)
	}
	if us >= 1e3 {
		return fmt.Sprintf("%.1fms", us/1e3)
	}
	if us >= 10 {
		return fmt.Sprintf("%.0fµs", us)
	}
	return fmt.Sprintf("%.2fµs", us)
}

// micro excludes the frame and launch benchmarks, whose async work makes
// their allocation counts vary run to run.
func micro(n string) bool { return !strings.HasPrefix(n, "Frame") && !strings.HasPrefix(n, "Launch/") }

func names(r *run, prefix string) []string {
	var out []string
	for _, n := range r.order {
		if strings.HasPrefix(n, prefix) {
			out = append(out, n)
		}
	}
	return out
}

func main() {
	basePath := flag.String("base", "", "bench output of the base revision")
	headPath := flag.String("head", "", "bench output of the revision under test")
	baseRef := flag.String("base-ref", "base", "label for the base column")
	headRef := flag.String("head-ref", "head", "label for the head column")
	jsonOut := flag.String("json", "", "also write head medians as JSON here")
	flag.Parse()
	base, err := parse(*basePath)
	if err != nil {
		fatal(err)
	}
	head, err := parse(*headPath)
	if err != nil {
		fatal(err)
	}
	if len(head.order) == 0 {
		fatal(fmt.Errorf("no benchmark results in %q", *headPath))
	}
	write(os.Stdout, base, head, *baseRef, *headRef, *basePath != "")
	if *jsonOut != "" {
		if err := writeJSON(*jsonOut, head); err != nil {
			fatal(err)
		}
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "report:", err)
	os.Exit(1)
}

func write(w io.Writer, base, head *run, baseRef, headRef string, compared bool) {
	var sec strings.Builder
	rows := 0
	p := func(format string, a ...any) { fmt.Fprintf(&sec, format, a...) }
	// row is content; a section with no rows is left out
	row := func(format string, a ...any) {
		rows++
		p(format, a...)
	}
	flush := func() {
		if rows > 0 {
			_, _ = io.WriteString(w, sec.String())
		}
		sec.Reset()
		rows = 0
	}
	title := func(format string, a ...any) { fmt.Fprintf(w, format, a...) }
	if compared {
		title("### pho benchmarks: `%s` → `%s`\n", baseRef, headRef)
		title("Report only, never blocks a merge. Timings come from a shared runner, so trust `~` and p-values over raw ms; the deterministic table is exact.\n\n")
	} else {
		title("### pho benchmarks: `%s`\n\n", headRef)
	}

	cols := func(c cmp, f func(float64) string, v string) string {
		if !compared {
			return f(c.head)
		}
		return fmt.Sprintf("%s | %s | %s", f(c.base), f(c.head), v)
	}
	header := func(first string, more ...string) {
		h := []string{first}
		if compared {
			h = append(h, baseRef, headRef, "Δ")
		} else {
			h = append(h, headRef)
		}
		h = append(h, more...)
		p("| %s |\n|%s\n", strings.Join(h, " | "), strings.Repeat("---|", len(h)))
	}

	p("**Deterministic** (exact counts, no noise)\n\n")
	header("metric")
	for _, n := range names(head, "Launch/") {
		if strings.Contains(n, "/local/") {
			c := compare(base, head, n, "requests")
			row("| API requests · %s | %s |\n", strings.Replace(strings.TrimPrefix(n, "Launch/"), "/local/", " ", 1), cols(c, num, c.exactVerdict()))
		}
	}
	for _, n := range names(head, "Frame") {
		c := compare(base, head, n, "B/frame")
		row("| terminal bytes/frame · %s | %s |\n", strings.TrimPrefix(n, "Frame"), cols(c, num, c.exactVerdict()))
	}
	var allocRows []string
	allocBase := false
	for _, n := range head.order {
		c := compare(base, head, n, "allocs/op")
		if !micro(n) || math.IsNaN(c.head) {
			continue
		}
		allocBase = allocBase || c.have
		if !compared || (c.have && c.base != 0 && math.Abs(c.delta()) >= 0.02) {
			allocRows = append(allocRows, fmt.Sprintf("| allocs/op · %s | %s |\n", n, cols(c, num, c.exactVerdict())))
		}
	}
	if compared {
		if len(allocRows) == 0 && allocBase {
			row("| allocs/op · micro benchmarks | unchanged within 2%% | | |\n")
		}
		for _, r := range allocRows {
			row("%s", r)
		}
	}
	p("\n")

	flush()
	p("**How it feels**: keypress → settled frame (p95 · p99 · frames over 16ms per run)\n\n")
	header("scenario", "")
	for _, n := range names(head, "Frame") {
		p95, p99 := compare(base, head, n, "p95-µs"), compare(base, head, n, "p99-µs")
		slow := compare(base, head, n, "slowframes/op")
		f := func(a, b, c float64) string { return fmt.Sprintf("%s · %s · %s", ms(a), ms(b), num(c)) }
		if compared {
			row("| %s | %s | %s | %s | |\n", strings.TrimPrefix(n, "Frame"), f(p95.base, p99.base, slow.base), f(p95.head, p99.head, slow.head), "p99 "+p99.verdict())
		} else {
			row("| %s | %s | |\n", strings.TrimPrefix(n, "Frame"), f(p95.head, p99.head, slow.head))
		}
	}
	p("\n")

	flush()
	p("**How fast it loads**: launch to first useful frame, cold / warm cache\n\n")
	lats := []string{"local", "fast", "ghes-vpn"}
	p("| scenario | %s |\n|---|%s\n", strings.Join(lats, " | "), strings.Repeat("---|", len(lats)))
	var scenarios []string
	seen := map[string]bool{}
	for _, n := range names(head, "Launch/") {
		sc := strings.Split(strings.TrimPrefix(n, "Launch/"), "/")[0]
		if !seen[sc] {
			seen[sc] = true
			scenarios = append(scenarios, sc)
		}
	}
	var loadChanges []string
	for _, sc := range scenarios {
		cells := []string{sc}
		for _, lat := range lats {
			var cell []string
			for _, phase := range []string{"cold", "warm"} {
				n := fmt.Sprintf("Launch/%s/%s/%s", sc, lat, phase)
				c := compare(base, head, n, "ttu-ms")
				cell = append(cell, ms(c.head*1e3))
				if compared && c.have && c.p < alpha {
					loadChanges = append(loadChanges, fmt.Sprintf("%s %s %s: %s → %s (%s)", sc, lat, phase, ms(c.base*1e3), ms(c.head*1e3), c.verdict()))
				}
			}
			cells = append(cells, strings.Join(cell, " / "))
		}
		row("| %s |\n", strings.Join(cells, " | "))
	}
	if compared {
		if len(loadChanges) == 0 {
			p("\nNo significant change against `%s`.\n", baseRef)
		} else {
			p("\nSignificant changes against `%s`:\n", baseRef)
			for _, l := range loadChanges {
				p("- %s\n", l)
			}
		}
	}
	p("\n")

	flush()
	if len(head.health) > 0 {
		p("**Health** (scaling slope: ~1 linear, ~2 quadratic; heap after 50 PR open/close cycles; p99 drift over a 20k-key soak)\n\n")
		keys := make([]string, 0, len(head.health))
		for k := range head.health {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		slope := regexp.MustCompile(`slope=([\d.]+)`)
		for _, k := range keys {
			line := head.health[k]
			if strings.HasPrefix(k, "SCALING ") {
				m := slope.FindStringSubmatch(line)
				was := ""
				if bm := slope.FindStringSubmatch(base.health[k]); bm != nil {
					was = " (was " + bm[1] + ")"
				}
				flag := ""
				if strings.Contains(line, "SUPERLINEAR") {
					flag = " ⚠️ superlinear"
				}
				if m != nil {
					row("- `%s` slope **%s**%s%s\n", strings.TrimPrefix(k, "SCALING "), m[1], was, flag)
				}
				continue
			}
			row("- %s: %s\n", strings.ToLower(strings.SplitN(k, " ", 2)[0]), line)
		}
		p("\n")
	}

	flush()
	if compared {
		type change struct {
			name string
			c    cmp
		}
		var changes []change
		total := 0
		for _, n := range head.order {
			if !micro(n) {
				continue
			}
			total++
			c := compare(base, head, n, "ns/op")
			if c.have && c.p < alpha && math.Abs(c.delta()) >= 0.03 {
				changes = append(changes, change{n, c})
			}
		}
		sort.Slice(changes, func(i, j int) bool { return math.Abs(changes[i].c.delta()) > math.Abs(changes[j].c.delta()) })
		row("**Micro benchmarks**: %d of %d changed significantly (p<%.2f, ≥3%%)\n\n", len(changes), total, alpha)
		if len(changes) > 0 {
			p("| benchmark | %s | %s | Δ |\n|---|---|---|---|\n", baseRef, headRef)
			for i, ch := range changes {
				if i == 15 {
					row("| … %d more | | | |\n", len(changes)-15)
					break
				}
				row("| %s | %s | %s | %s |\n", ch.name, ms(ch.c.base/1e3), ms(ch.c.head/1e3), ch.c.verdict())
			}
			p("\n")
		}
	}
	if !compared {
		p("**Micro benchmarks**\n\n| benchmark | time/op | allocs/op |\n|---|---|---|\n")
		for _, n := range head.order {
			if !micro(n) {
				continue
			}
			u := head.samples[n]
			row("| %s | %s | %s |\n", n, ms(median(u["ns/op"])/1e3), num(median(u["allocs/op"])))
		}
		p("\n")
	}
	flush()
}

// writeJSON stores head medians per benchmark and unit, for history.
func writeJSON(path string, head *run) error {
	out := map[string]map[string]float64{}
	for n, units := range head.samples {
		out[n] = map[string]float64{}
		for u, xs := range units {
			out[n][u] = median(xs)
		}
	}
	b, err := json.MarshalIndent(map[string]any{"benchmarks": out, "health": head.health}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}
