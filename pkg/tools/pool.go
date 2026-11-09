package tools

import (
	"context"
	"sync"
)

const DefaultMaxParallelToolCalls = 10

func RunParallel[T any](ctx context.Context, tasks []func(context.Context) (T, error), maxConcurrent int) ([]T, error) {
	if maxConcurrent < 1 {
		maxConcurrent = 1
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	results := make([]T, len(tasks))
	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup
	var errOnce sync.Once
	var firstErr error

	for i, task := range tasks {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			break
		}
		if ctx.Err() != nil {
			<-sem
			break
		}
		wg.Add(1)
		go func(idx int, fn func(context.Context) (T, error)) {
			defer wg.Done()
			defer func() { <-sem }()
			v, err := fn(ctx)
			if err != nil {
				errOnce.Do(func() {
					firstErr = err
					cancel()
				})
				return
			}
			results[idx] = v
		}(i, task)
	}
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	return results, nil
}
