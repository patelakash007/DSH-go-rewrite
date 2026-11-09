//

//

//

// sup := jobs.New()
// sup.Go("jsonl-shard-3", func(ctx context.Context) { j.shardWriter(ctx, 3) })
// sup.Go("fsnotify",           func(ctx context.Context) { watchLoop(ctx, dir) })
// if err := sup.Shutdown(ctx); err != nil { ... }
package jobs

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type Supervisor struct {
	mu     sync.Mutex
	ctx    context.Context
	cancel context.CancelFunc

	workers map[string]*workerState
	closed  bool
}

type workerState struct {
	cancel context.CancelFunc
	done   chan struct{}
}

func New() *Supervisor {
	ctx, cancel := context.WithCancel(context.Background())
	return &Supervisor{ctx: ctx, cancel: cancel, workers: map[string]*workerState{}}
}

func (s *Supervisor) Go(name string, fn func(ctx context.Context)) bool {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return false
	}

	key := name
	if _, exists := s.workers[key]; exists {
		for i := 1; ; i++ {
			cand := fmt.Sprintf("%s#%d", name, i)
			if _, ok := s.workers[cand]; !ok {
				key = cand
				break
			}
		}
	}
	wctx, wcancel := context.WithCancel(s.ctx)
	st := &workerState{cancel: wcancel, done: make(chan struct{})}
	s.workers[key] = st
	s.mu.Unlock()

	go func() {
		defer close(st.done)
		defer wcancel()
		fn(wctx)
	}()
	return true
}

func (s *Supervisor) WorkerCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.workers)
}

func (s *Supervisor) Names() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.workers))
	for n := range s.workers {
		out = append(out, n)
	}
	return out
}

func (s *Supervisor) CancelWorker(name string) {
	s.mu.Lock()
	st, ok := s.workers[name]
	s.mu.Unlock()
	if ok && st != nil {
		st.cancel()
	}
}

func (s *Supervisor) WaitWorker(ctx context.Context, name string) bool {
	s.mu.Lock()
	st, ok := s.workers[name]
	s.mu.Unlock()
	if !ok || st == nil {
		return true
	}
	select {
	case <-st.done:
		return true
	case <-ctx.Done():
		return false
	}
}

func (s *Supervisor) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.cancel()
	states := make([]*workerState, 0, len(s.workers))
	for _, st := range s.workers {
		states = append(states, st)
	}
	s.mu.Unlock()

	var wg sync.WaitGroup
	var mu sync.Mutex
	var failed []string
	for _, st := range states {
		wg.Add(1)
		go func(x *workerState, name string) {
			defer wg.Done()
			select {
			case <-x.done:
			case <-ctx.Done():
				mu.Lock()
				failed = append(failed, name)
				mu.Unlock()
			}
		}(st, "") // name not needed in error; we collect only on timeout
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-ctx.Done():
		mu.Lock()
		list := failed
		mu.Unlock()
		if len(list) == 0 {
			return ctx.Err()
		}
		return fmt.Errorf("jobs: shutdown timeout, %d worker(s) not exited", len(list))
	}
	return nil
}

func (s *Supervisor) Healthy(ctx context.Context) bool {
	s.mu.Lock()
	states := make([]*workerState, 0, len(s.workers))
	for _, st := range s.workers {
		states = append(states, st)
	}
	s.mu.Unlock()

	for _, st := range states {
		select {
		case <-st.done:
		default:
			return true
		}
	}
	return false
}

func (s *Supervisor) Close() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = s.Shutdown(ctx)
}
