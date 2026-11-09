package schedule

import (
	"errors"
	"sync"
	"time"
)

const MinEveryInterval = 5 * time.Minute

type Kind string

const (
	KindAfter Kind = "after"
	KindAt    Kind = "at"
	KindEvery Kind = "every"
)

var ErrEmptyPrompt = errors.New("schedule: empty prompt")

var ErrNotFuture = errors.New("schedule: target is not in the future")

var ErrIntervalTooShort = errors.New("schedule: every interval below minimum")

type Entry struct {
	ID       string
	Kind     Kind
	Prompt   string
	NextAt   time.Time
	Interval time.Duration
}

type Dispatch struct {
	ID     string
	Prompt string
}

type Scheduler struct {
	mu      sync.Mutex
	entries map[string]*Entry
	timers  map[string]*time.Timer
	out     chan Dispatch
	now     func() time.Time
}

func New() *Scheduler {
	return &Scheduler{
		entries: map[string]*Entry{},
		timers:  map[string]*time.Timer{},
		out:     make(chan Dispatch, 16),
		now:     time.Now,
	}
}

func (s *Scheduler) SetNow(f func() time.Time) {
	s.mu.Lock()
	s.now = f
	s.mu.Unlock()
}

func (s *Scheduler) Out() <-chan Dispatch { return s.out }

func (s *Scheduler) arm(id string, at time.Time) {
	d := time.Until(at)
	t := time.AfterFunc(d, func() { s.fire(id) })
	s.timers[id] = t
}

func (s *Scheduler) After(id, prompt string, delay time.Duration) error {
	if prompt == "" {
		return ErrEmptyPrompt
	}
	if delay <= 0 {
		return ErrNotFuture
	}
	return s.add(&Entry{ID: id, Kind: KindAfter, Prompt: prompt, NextAt: s.now().Add(delay)})
}

func (s *Scheduler) At(id, prompt string, at time.Time) error {
	if prompt == "" {
		return ErrEmptyPrompt
	}
	if !at.After(s.now()) {
		return ErrNotFuture
	}
	return s.add(&Entry{ID: id, Kind: KindAt, Prompt: prompt, NextAt: at})
}

func (s *Scheduler) Every(id, prompt string, interval time.Duration) error {
	if prompt == "" {
		return ErrEmptyPrompt
	}
	if interval < MinEveryInterval {
		return ErrIntervalTooShort
	}
	e := &Entry{ID: id, Kind: KindEvery, Prompt: prompt, NextAt: s.now().Add(interval), Interval: interval}
	return s.add(e)
}

func (s *Scheduler) add(e *Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[e.ID] = e
	s.arm(e.ID, e.NextAt)
	return nil
}

func (s *Scheduler) fire(id string) {
	s.mu.Lock()
	e, ok := s.entries[id]
	if !ok {
		s.mu.Unlock()
		return
	}
	s.out <- Dispatch{ID: id, Prompt: e.Prompt}
	if e.Kind == KindEvery {
		e.NextAt = s.now().Add(e.Interval)
		s.arm(id, e.NextAt)
	} else {
		delete(s.entries, id)
		delete(s.timers, id)
	}
	s.mu.Unlock()
}

func (s *Scheduler) Cancel(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.timers[id]
	if ok {
		t.Stop()
	}
	_, exists := s.entries[id]
	delete(s.entries, id)
	delete(s.timers, id)
	return exists
}

func (s *Scheduler) List() []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Entry, 0, len(s.entries))
	for _, e := range s.entries {
		out = append(out, *e)
	}
	return out
}

func (s *Scheduler) Shutdown() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range s.timers {
		t.Stop()
	}
	s.entries = map[string]*Entry{}
	s.timers = map[string]*time.Timer{}
}
