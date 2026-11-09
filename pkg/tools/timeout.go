//

package tools

import (
	"context"
	"errors"
	"time"
)

const ToolTimeoutCode = "TOOL_TIMEOUT"

var ErrToolTimeout = errors.New("tool call timed out")

type TimeoutError struct {
	Code   string
	Budget time.Duration
	Cause  error
}

func (e *TimeoutError) Error() string {
	return "tool call timed out after " + e.Budget.String()
}

func (e *TimeoutError) Unwrap() error { return e.Cause }

func WrapTimeout(exec ToolExecuteFunc, timeoutMs int) ToolExecuteFunc {
	if exec == nil || timeoutMs <= 0 {
		return exec
	}
	budget := time.Duration(timeoutMs) * time.Millisecond
	return func(ctx context.Context, input map[string]any) (any, error) {
		timerCtx, cancel := context.WithTimeout(ctx, budget)
		defer cancel()

		type outcome struct {
			value any
			err   error
		}
		done := make(chan outcome, 1)
		go func() {
			v, e := exec(timerCtx, input)
			done <- outcome{v, e}
		}()

		select {
		case o := <-done:
			return o.value, o.err
		case <-timerCtx.Done():

			if errors.Is(timerCtx.Err(), context.DeadlineExceeded) && ctx.Err() == nil {
				return nil, &TimeoutError{Code: ToolTimeoutCode, Budget: budget, Cause: ErrToolTimeout}
			}
			return nil, ctx.Err()
		}
	}
}
