// Package jobs schedules deduplicated foreground and background work.
package jobs

import (
	"context"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
)

// JobScheduler runs jobs with deduplication, cancellation, and bounded concurrency.
type JobScheduler interface {
	Run(job JobSpec)
	CancelPrefix(prefix string)
	InFlight(key string) bool
}

// Priority controls scheduling order.
type Priority int

const (
	PriorityBackground Priority = iota
	PriorityForeground
)

// JobSpec describes a single schedulable unit of work.
type JobSpec struct {
	Key        string
	Host       string
	Priority   Priority
	Run        func(context.Context) tea.Msg
	OnComplete func(tea.Msg)
}

// Option configures a Scheduler.
type Option func(*Scheduler)

// WithLimits overrides the global and per-host concurrency limits.
func WithLimits(global, perHost int) Option {
	return func(s *Scheduler) {
		if global > 0 {
			s.globalLimit = global
		}
		if perHost > 0 {
			s.hostLimit = perHost
		}
	}
}

type jobEntry struct {
	spec      JobSpec
	seq       uint64
	ctx       context.Context
	cancel    context.CancelFunc
	cancelled bool
}

// Scheduler implements JobScheduler.
type Scheduler struct {
	mu            sync.Mutex
	cond          *sync.Cond
	rootCtx       context.Context
	rootCancel    context.CancelFunc
	pending       []*jobEntry
	running       map[string]*jobEntry
	inFlight      map[string]*jobEntry
	hostRunning   map[string]int
	globalRunning int
	globalLimit   int
	hostLimit     int
	seq           uint64
	closed        bool
}

const (
	defaultGlobalLimit = 6
	defaultHostLimit   = 2
)

func New(opts ...Option) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Scheduler{
		rootCtx:     ctx,
		rootCancel:  cancel,
		running:     make(map[string]*jobEntry),
		inFlight:    make(map[string]*jobEntry),
		hostRunning: make(map[string]int),
		globalLimit: defaultGlobalLimit,
		hostLimit:   defaultHostLimit,
	}
	s.cond = sync.NewCond(&s.mu)
	for _, opt := range opts {
		opt(s)
	}
	go s.loop()
	return s
}

// Run enqueues a job unless a job with the same key is already queued or running.
func (s *Scheduler) Run(job JobSpec) {
	if job.Run == nil || job.Key == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	if _, exists := s.inFlight[job.Key]; exists {
		return
	}

	ctx, cancel := context.WithCancel(s.rootCtx)
	entry := &jobEntry{
		spec:   job,
		seq:    s.seq,
		ctx:    ctx,
		cancel: cancel,
	}
	s.seq++
	s.pending = append(s.pending, entry)
	s.inFlight[job.Key] = entry
	s.cond.Signal()
}

// CancelPrefix cancels queued and running jobs whose keys match prefix.
func (s *Scheduler) CancelPrefix(prefix string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return
	}

	kept := s.pending[:0]
	for _, entry := range s.pending {
		if strings.HasPrefix(entry.spec.Key, prefix) {
			entry.cancelled = true
			entry.cancel()
			delete(s.inFlight, entry.spec.Key)
			continue
		}
		kept = append(kept, entry)
	}
	s.pending = kept

	for key, entry := range s.running {
		if strings.HasPrefix(key, prefix) {
			entry.cancelled = true
			entry.cancel()
		}
	}

	s.cond.Broadcast()
}

// InFlight reports whether a key is queued or running.
func (s *Scheduler) InFlight(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.inFlight[key]
	return ok
}

// Close stops the scheduler and cancels all queued/running jobs.
func (s *Scheduler) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	for _, entry := range s.pending {
		entry.cancelled = true
		entry.cancel()
		delete(s.inFlight, entry.spec.Key)
	}
	for _, entry := range s.running {
		entry.cancelled = true
		entry.cancel()
	}
	s.pending = nil
	s.cond.Broadcast()
	s.mu.Unlock()
	s.rootCancel()
}

func (s *Scheduler) loop() {
	for {
		s.mu.Lock()
		for !s.closed {
			idx := s.bestRunnableIndexLocked()
			if idx >= 0 {
				entry := s.pending[idx]
				s.pending = append(s.pending[:idx], s.pending[idx+1:]...)
				s.startLocked(entry)
				s.mu.Unlock()
				go s.execute(entry)
				goto next
			}
			if len(s.pending) == 0 {
				s.cond.Wait()
				continue
			}
			s.cond.Wait()
		}
		s.mu.Unlock()
		return
	next:
	}
}

func (s *Scheduler) bestRunnableIndexLocked() int {
	if s.globalRunning >= s.globalLimit {
		return -1
	}

	bestIdx := -1
	var best *jobEntry
	for i, entry := range s.pending {
		if s.globalRunning >= s.globalLimit {
			break
		}
		if s.hostRunning[entry.spec.Host] >= s.hostLimit {
			continue
		}
		if best == nil || entry.spec.Priority > best.spec.Priority || (entry.spec.Priority == best.spec.Priority && entry.seq < best.seq) {
			best = entry
			bestIdx = i
		}
	}
	return bestIdx
}

func (s *Scheduler) startLocked(entry *jobEntry) {
	s.running[entry.spec.Key] = entry
	s.hostRunning[entry.spec.Host]++
	s.globalRunning++
}

func (s *Scheduler) execute(entry *jobEntry) {
	msg := entry.spec.Run(entry.ctx)

	var complete func(tea.Msg)

	s.mu.Lock()
	if current, ok := s.running[entry.spec.Key]; ok && current == entry {
		delete(s.running, entry.spec.Key)
		if n := s.hostRunning[entry.spec.Host]; n > 0 {
			s.hostRunning[entry.spec.Host] = n - 1
		}
		if s.globalRunning > 0 {
			s.globalRunning--
		}
	}
	delete(s.inFlight, entry.spec.Key)
	if !entry.cancelled {
		complete = entry.spec.OnComplete
	}
	s.cond.Broadcast()
	s.mu.Unlock()

	if complete != nil {
		complete(msg)
	}
}
