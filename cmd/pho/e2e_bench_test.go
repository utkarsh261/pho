package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
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

var ansiSeq = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\][^\x07]*\x07`)

// screen records what the program writes and when a marker first shows up.
type screen struct {
	mu     sync.Mutex
	start  time.Time
	first  time.Duration
	text   strings.Builder
	wants  map[string]chan time.Duration
	closed map[string]bool
}

func newScreen() *screen {
	return &screen{start: time.Now(), wants: map[string]chan time.Duration{}, closed: map[string]bool{}}
}

func (s *screen) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	at := time.Since(s.start)
	if s.first == 0 {
		s.first = at
	}
	s.text.WriteString(ansiSeq.ReplaceAllString(string(p), ""))
	s.check(at)
	return len(p), nil
}

func (s *screen) check(at time.Duration) {
	text := s.text.String()
	for m, ch := range s.wants {
		if !s.closed[m] && strings.Contains(text, m) {
			s.closed[m] = true
			ch <- at
		}
	}
}

// since resets the stream so a marker counts only from now on.
func (s *screen) since() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.text.Reset()
	return time.Now()
}

func (s *screen) wait(tb testing.TB, marker string, timeout time.Duration) time.Duration {
	tb.Helper()
	ch := make(chan time.Duration, 1)
	s.mu.Lock()
	s.wants[marker] = ch
	s.closed[marker] = false
	s.check(time.Since(s.start))
	s.mu.Unlock()
	select {
	case at := <-ch:
		return at
	case <-time.After(timeout):
		s.mu.Lock()
		tail := s.text.String()
		s.mu.Unlock()
		tb.Fatalf("%q not on screen after %v; last output:\n%s", marker, timeout, tail[max(0, len(tail)-2000):])
		return 0
	}
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
// after auth.
func (r launchRun) start(tb testing.TB) (*tea.Program, *screen, chan struct{}) {
	tb.Helper()
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
	scr := newScreen()
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

func ts64457Diff() string {
	for _, f := range difftest.RawDiffs() {
		if f.Name == "ts64457" {
			return f.Raw
		}
	}
	panic("ts64457 fixture missing")
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
	{name: "pr-ts64457", repos: 1, perRepo: 5, prNumber: 3, diff: ts64457Diff, files: "ts64457", marker: "generate-options.ts"},
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
			p, scr, done := run.start(b)
			scr.wait(b, sc.marker, 30*time.Second)
			quiesce(srv, 300*time.Millisecond, 30*time.Second)
			p.Quit()
			<-done
		}
		srv.Reset()
		b.StartTimer()

		p, scr, done := run.start(b)
		at := scr.wait(b, sc.marker, 60*time.Second)

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
