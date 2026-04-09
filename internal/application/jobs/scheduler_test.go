package jobs

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestScheduler_DedupeByKey(t *testing.T) {
	s := New(WithLimits(1, 1))
	t.Cleanup(s.Close)

	started := make(chan struct{}, 1)
	release := make(chan struct{})
	var runs atomic.Int32

	s.Run(JobSpec{
		Key:  "repo/1",
		Host: "github.com",
		Run: func(context.Context) tea.Msg {
			runs.Add(1)
			started <- struct{}{}
			<-release
			return nil
		},
	})

	<-started
	s.Run(JobSpec{
		Key:  "repo/1",
		Host: "github.com",
		Run: func(context.Context) tea.Msg {
			runs.Add(1)
			return nil
		},
	})

	if got := runs.Load(); got != 1 {
		t.Fatalf("duplicate job ran %d times, want 1", got)
	}

	close(release)
	waitFor(t, time.Second, func() bool { return !s.InFlight("repo/1") })
}

func TestScheduler_CancelPrefix(t *testing.T) {
	s := New(WithLimits(1, 1))
	t.Cleanup(s.Close)

	started := make(chan string, 3)
	release := make(chan struct{})
	var mu sync.Mutex
	var order []string

	record := func(key string) func(context.Context) tea.Msg {
		return func(context.Context) tea.Msg {
			started <- key
			<-release
			mu.Lock()
			order = append(order, key)
			mu.Unlock()
			return nil
		}
	}

	s.Run(JobSpec{Key: "repo:a/1", Host: "github.com", Run: record("repo:a/1")})
	s.Run(JobSpec{Key: "repo:a/2", Host: "github.com", Run: record("repo:a/2")})
	s.Run(JobSpec{Key: "repo:b/1", Host: "github.com", Run: record("repo:b/1")})

	if got := <-started; got != "repo:a/1" {
		t.Fatalf("first started job = %q, want repo:a/1", got)
	}

	s.CancelPrefix("repo:a")
	close(release)

	if got := <-started; got != "repo:b/1" {
		t.Fatalf("second started job = %q, want repo:b/1", got)
	}

	waitFor(t, time.Second, func() bool { return !s.InFlight("repo:a/1") && !s.InFlight("repo:a/2") && !s.InFlight("repo:b/1") })

	mu.Lock()
	defer mu.Unlock()
	if len(order) != 2 || order[0] != "repo:a/1" || order[1] != "repo:b/1" {
		t.Fatalf("completed order = %v, want [repo:a/1 repo:b/1]", order)
	}
	assertNoStringSignal(t, started, 100*time.Millisecond)
}

func TestScheduler_ConcurrencyCap(t *testing.T) {
	s := New(WithLimits(2, 2))
	t.Cleanup(s.Close)

	started := make(chan struct{}, 10)
	release := make(chan struct{})
	var running atomic.Int32
	var maxRunning atomic.Int32

	job := func(key string) JobSpec {
		return JobSpec{
			Key:  key,
			Host: "github.com",
			Run: func(context.Context) tea.Msg {
				cur := running.Add(1)
				updateMax(&maxRunning, cur)
				started <- struct{}{}
				<-release
				running.Add(-1)
				return nil
			},
		}
	}

	for i := 0; i < 10; i++ {
		s.Run(job("repo/1-" + string(rune('a'+i))))
	}

	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for initial jobs to start")
		}
	}

	assertNoStructSignal(t, started, 200*time.Millisecond)
	if got := maxRunning.Load(); got != 2 {
		t.Fatalf("max running = %d, want 2", got)
	}

	close(release)
	waitFor(t, time.Second, func() bool { return !s.InFlight("repo/1-a") })
}

func TestScheduler_PriorityOrdering(t *testing.T) {
	s := New(WithLimits(1, 1))
	t.Cleanup(s.Close)

	started := make(chan string, 4)
	releaseFirst := make(chan struct{})
	releaseOthers := make(chan struct{})

	s.Run(JobSpec{
		Key:      "background/1",
		Host:     "github.com",
		Priority: PriorityBackground,
		Run: func(context.Context) tea.Msg {
			started <- "background/1"
			<-releaseFirst
			return nil
		},
	})

	if got := <-started; got != "background/1" {
		t.Fatalf("first started job = %q, want background/1", got)
	}

	s.Run(JobSpec{
		Key:      "background/2",
		Host:     "github.com",
		Priority: PriorityBackground,
		Run: func(context.Context) tea.Msg {
			started <- "background/2"
			<-releaseOthers
			return nil
		},
	})

	s.Run(JobSpec{
		Key:      "foreground/1",
		Host:     "github.com",
		Priority: PriorityForeground,
		Run: func(context.Context) tea.Msg {
			started <- "foreground/1"
			<-releaseOthers
			return nil
		},
	})

	close(releaseFirst)

	if got := <-started; got != "foreground/1" {
		t.Fatalf("second started job = %q, want foreground/1", got)
	}

	close(releaseOthers)
	waitFor(t, time.Second, func() bool { return !s.InFlight("background/2") && !s.InFlight("foreground/1") })
}

func waitFor(t *testing.T, timeout time.Duration, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("timed out waiting for condition")
}

func assertNoStringSignal(t *testing.T, ch <-chan string, timeout time.Duration) {
	t.Helper()
	select {
	case <-ch:
		t.Fatal("unexpected extra job start")
	case <-time.After(timeout):
	}
}

func assertNoStructSignal(t *testing.T, ch <-chan struct{}, timeout time.Duration) {
	t.Helper()
	select {
	case <-ch:
		t.Fatal("unexpected extra job start")
	case <-time.After(timeout):
	}
}

func updateMax(dst *atomic.Int32, cur int32) {
	for {
		prev := dst.Load()
		if cur <= prev {
			return
		}
		if dst.CompareAndSwap(prev, cur) {
			return
		}
	}
}
