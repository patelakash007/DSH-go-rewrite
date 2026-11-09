//

//

//

package waterfall

import (
	"context"
	"fmt"
)

type NextFunc func() error

type Handler[T any] func(payload *T, next NextFunc) error

type Chain[T any] struct {
	handlers []Handler[T]
}

func New[T any](handlers ...Handler[T]) *Chain[T] {
	return &Chain[T]{handlers: append([]Handler[T]{}, handlers...)}
}

func (c *Chain[T]) Use(handlers ...Handler[T]) *Chain[T] {
	if c.handlers == nil {
		panic("waterfall: cannot Use() on a Chain built with zero value; use New()")
	}
	c.handlers = append(c.handlers, handlers...)
	return c
}

func (c *Chain[T]) Run(ctx context.Context, payload *T) error {
	if ctx == nil {
		return fmt.Errorf("waterfall: nil context")
	}
	return c.runFrom(ctx, 0, payload)
}

func (c *Chain[T]) runFrom(ctx context.Context, index int, payload *T) error {

	if index >= len(c.handlers) {
		return nil
	}

	current := c.handlers[index]

	called := false
	var nextErr error
	next := func() error {
		if called {
			panic("waterfall: next() called more than once in a single handler")
		}
		called = true
		nextErr = c.runFrom(ctx, index+1, payload)
		return nextErr
	}

	err := current(payload, next)
	if err != nil {
		return err
	}

	if !called {
		return nil
	}
	return nextErr
}

func (c *Chain[T]) RunSafe(ctx context.Context, payload *T) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("waterfall: panic in chain: %v", p)
		}
	}()
	return c.Run(ctx, payload)
}
