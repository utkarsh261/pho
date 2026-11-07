package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/utkarsh261/pho/internal/bench"
	"github.com/utkarsh261/pho/internal/config"
	"github.com/utkarsh261/pho/internal/diff/difftest"
	githubpkg "github.com/utkarsh261/pho/internal/github"
	pholog "github.com/utkarsh261/pho/internal/log"
	"github.com/utkarsh261/pho/internal/state"
	"github.com/utkarsh261/pho/internal/testutil/fakegh"
)

// screen records what the program writes, minus escape sequences, and when
// marker first shows up. Write does work proportional to the chunk, since it
// runs on the renderer's path.
type screen struct {
	mu     sync.Mutex
	start  time.Time
	first  time.Duration
	text   []byte
	esc    escState
	marker []byte
	found  chan time.Duration
}

func newScreen(marker string) *screen {
	return &screen{start: time.Now(), marker: []byte(marker), found: make(chan time.Duration, 1)}
}

func (s *screen) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	at := time.Since(s.start)
	if s.first == 0 {
		s.first = at
	}
	old := len(s.text)
	s.text = s.esc.strip(s.text, p)
	// only text that this chunk could have completed
	if s.marker != nil && bytes.Contains(s.text[max(0, old-len(s.marker)+1):], s.marker) {
		s.found <- at
		s.marker = nil
	}
	return len(p), nil
}

func (s *screen) wait(tb testing.TB, timeout time.Duration) time.Duration {
	tb.Helper()
	select {
	case at := <-s.found:
		return at
	case <-time.After(timeout):
		s.mu.Lock()
		tail := string(s.text[max(0, len(s.text)-2000):])
		s.mu.Unlock()
		tb.Fatalf("marker not on screen after %v; last output:\n%s", timeout, tail)
		return 0
	}
}

// escState strips CSI, OSC and short escape sequences, carrying state across
// writes so a sequence split between two chunks is still removed.
type escState int

const (
	escNone escState = iota
	escStart
	escCSI
	escOSC
	escOSCEsc
	escCharset
)

func (st *escState) strip(dst, p []byte) []byte {
	for _, c := range p {
		switch *st {
		case escNone:
			if c == 0x1b {
				*st = escStart
			} else {
				dst = append(dst, c)
			}
		case escStart:
			switch c {
			case '[':
				*st = escCSI
			case ']':
				*st = escOSC
			case '(', ')':
				*st = escCharset
			default:
				*st = escNone
			}
		case escCSI:
			if c >= 0x40 && c <= 0x7e {
				*st = escNone
			}
		case escOSC:
			switch c {
			case 0x07:
				*st = escNone
			case 0x1b:
				*st = escOSCEsc
			}
		case escOSCEsc, escCharset:
			*st = escNone
		}
	}
	return dst
}

type launchRun struct {
	srv      *fakegh.Server
	dir      string
	prNumber int
	root     string
}

func isolate(tb testing.TB, dir string) {
	for _, k := range []string{"HOME", "XDG_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_STATE_HOME", "XDG_DATA_HOME"} {
		tb.Setenv(k, filepath.Join(dir, strings.ToLower(k)))
	}
	tb.Setenv("PHO_NO_UPDATE_CHECK", "1")
}

// start launches pho against the fake server in dir, the way main does
// after auth. The screen's clock starts before config and wiring, so their
// cost counts toward the launch.
func (r launchRun) start(tb testing.TB, marker string) (*tea.Program, *screen, chan struct{}) {
	tb.Helper()
	scr := newScreen(marker)
	cfg, err := config.Load(filepath.Join(r.dir, "config.toml"))
	if err != nil {
		tb.Fatal(err)
	}
	cfg.Cache.Dir = filepath.Join(r.dir, "cache")
	cfg.Logging.File = filepath.Join(r.dir, "pho.log")
	cfg.Paths.StateFile = filepath.Join(r.dir, "state.json")
	cfg.Updates.Check, cfg.Updates.WhatsNew = false, false
	model := buildModel(launch{
		cfg:      cfg,
		inv:      invocation{PRNumber: r.prNumber},
		rootDir:  r.root,
		profiles: []githubpkg.GitHubHostProfile{r.srv.Profile()},
		logger:   pholog.New(cfg.Logging.File, "info"),
		state:    state.NewStore(cfg.Paths.StateFile),
	})
	in, _ := io.Pipe()
	p := tea.NewProgram(model, tea.WithInput(in), tea.WithOutput(scr), tea.WithoutSignalHandler())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = p.Run()
	}()
	p.Send(tea.WindowSizeMsg{Width: 200, Height: 50})
	return p, scr, done
}

// quiesce waits until the server has seen no request for idle, so the
// request count covers everything a launch triggers.
func quiesce(srv *fakegh.Server, idle, limit time.Duration) {
	deadline := time.Now().Add(limit)
	last, since := srv.Total(), time.Now()
	for time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
		if n := srv.Total(); n != last {
			last, since = n, time.Now()
		} else if time.Since(since) >= idle {
			return
		}
	}
}

func dirSizeKB(dir string) float64 {
	var n int64
	_ = filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			n += info.Size()
		}
		return nil
	})
	return float64(n) / 1024
}

type launchScenario struct {
	name     string
	repos    int
	perRepo  int
	diff     func() string
	files    string
	prNumber int
	marker   string // on screen once the dashboard rows or the PR's files are
}

var launchScenarios = []launchScenario{
	{name: "dashboard-5-repos", repos: 5, perRepo: 20, marker: "feature/"},
	{name: "dashboard-40-repos", repos: 40, perRepo: 50, marker: "feature/"},
	{name: "pr-small", repos: 1, perRepo: 5, prNumber: 3, diff: func() string { return bench.SynthSmall(5, 48) }, marker: "pkg/mod4/file4.go"},
	{name: "pr-ts64457", repos: 1, perRepo: 5, prNumber: 3, diff: func() string { return difftest.Raw("ts64457") }, files: "ts64457", marker: "generate-options.ts"},
}

// BenchmarkLaunch measures a whole launch against an in-process GitHub:
// first frame, first useful frame, API requests until the app goes quiet,
// and the cache it leaves behind. Warm runs reuse the cold run's cache.
func BenchmarkLaunch(b *testing.B) {
	for _, sc := range launchScenarios {
		for _, lat := range []fakegh.Latency{fakegh.Local, fakegh.Fast, fakegh.GHESVPN} {
			for _, warm := range []bool{false, true} {
				phase := "cold"
				if warm {
					phase = "warm"
				}
				b.Run(fmt.Sprintf("%s/%s/%s", sc.name, lat.Name, phase), func(b *testing.B) {
					runLaunch(b, sc, lat, warm)
				})
			}
		}
	}
}

func runLaunch(b *testing.B, sc launchScenario, lat fakegh.Latency, warm bool) {
	opts := fakegh.Options{Repos: sc.repos, PRsPerRepo: sc.perRepo, Latency: lat, Seed: 1}
	if sc.diff != nil {
		opts.Diff = sc.diff()
	}
	if sc.files != "" {
		opts.FilesJSON = difftest.FilesJSON(sc.files)
	} else {
		opts.FilesJSON = []byte("[]")
	}
	srv := fakegh.New(opts)
	b.Cleanup(srv.Close)

	var ttff, ttu time.Duration
	var reqs, cacheKB float64
	for b.Loop() {
		b.StopTimer()
		dir := b.TempDir()
		isolate(b, dir)
		root := filepath.Join(dir, "src")
		if err := srv.WriteRoot(root); err != nil {
			b.Fatal(err)
		}
		if sc.prNumber > 0 {
			root = filepath.Join(root, srv.Repos()[0].Name)
		}
		run := launchRun{srv: srv, dir: dir, prNumber: sc.prNumber, root: root}
		if warm {
			p, scr, done := run.start(b, sc.marker)
			scr.wait(b, 30*time.Second)
			quiesce(srv, 300*time.Millisecond, 30*time.Second)
			p.Quit()
			<-done
		}
		srv.Reset()
		b.StartTimer()

		p, scr, done := run.start(b, sc.marker)
		at := scr.wait(b, 60*time.Second)

		b.StopTimer()
		quiesce(srv, 300*time.Millisecond, 30*time.Second)
		p.Quit()
		<-done
		scr.mu.Lock()
		ttff += scr.first
		scr.mu.Unlock()
		ttu += at
		reqs += float64(srv.Total())
		cacheKB += dirSizeKB(filepath.Join(dir, "cache"))
		if testing.Verbose() {
			b.Logf("REQUESTS %s %s", b.Name(), srv.Summary())
		}
		b.StartTimer()
	}
	n := float64(b.N)
	b.ReportMetric(float64(ttff.Microseconds())/1e3/n, "ttff-ms")
	b.ReportMetric(float64(ttu.Microseconds())/1e3/n, "ttu-ms")
	b.ReportMetric(reqs/n, "requests")
	b.ReportMetric(cacheKB/n, "cache-KB")
}
