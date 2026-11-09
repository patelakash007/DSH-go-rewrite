package attachment

import "sync"

type Limiter struct {
	slots chan struct{}

	mu      sync.Mutex
	waiters []chan struct{}
}

func NewLimiter(concurrency int) *Limiter {
	if concurrency < 1 {
		concurrency = 1
	}
	return &Limiter{slots: make(chan struct{}, concurrency)}
}

func (l *Limiter) Acquire() { l.slots <- struct{}{} }

func (l *Limiter) TryAcquire() bool {
	select {
	case l.slots <- struct{}{}:
		return true
	default:
		return false
	}
}

func (l *Limiter) Release() { <-l.slots }

func (l *Limiter) Active() int { return len(l.slots) }

func (l *Limiter) Run(task func()) {
	l.Acquire()
	defer l.Release()
	task()
}
