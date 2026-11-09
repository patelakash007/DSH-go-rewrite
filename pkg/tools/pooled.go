//

package tools

import (
	"context"
	"sync"
)

var execContextPool = sync.Pool{
	New: func() any { return &ExecContext{Meta: map[string]any{}} },
}

func clearMeta(m map[string]any) {
	for k := range m {
		delete(m, k)
	}
}

func (p *Pipeline) RunPooled(ctx context.Context, req *ToolCallRequest, tool *Tool) *ToolCallResult {

	ec := execContextPool.Get().(*ExecContext)
	ec.Request = req
	ec.Signal = SignalContinue
	ec.Denied = false
	ec.Ctx = ctx
	if ec.Result == nil {
		ec.Result = &ToolCallResult{}
	}
	if ec.Meta == nil {
		ec.Meta = map[string]any{}
	} else {
		clearMeta(ec.Meta)
	}

	inner := ec.Result
	inner.CallID = req.CallID
	inner.IsError = false
	inner.Value = nil
	inner.Error = ""

	_ = p.pre.Run(ctx, ec)
	if ec.Denied {
		inner.IsError = true
		inner.Error = "denied by pre-execute middleware"
		out := copyResult(inner)
		releaseExecContext(ec)
		return out
	}

	_ = p.execute.Run(ctx, ec)
	if ec.Signal == SignalCancel {
		inner.IsError = true
	}

	_ = p.post.Run(ctx, ec)
	if ec.Denied {
		inner.IsError = true
		if inner.Error == "" {
			inner.Error = "blocked by post-execute middleware"
		}
	}

	_ = p.result.Run(ctx, ec)

	out := copyResult(inner)
	releaseExecContext(ec)
	return out
}

func copyResult(r *ToolCallResult) *ToolCallResult {
	if r == nil {
		return nil
	}
	return &ToolCallResult{
		CallID:  r.CallID,
		IsError: r.IsError,
		Value:   r.Value,
		Error:   r.Error,
	}
}

func releaseExecContext(ec *ExecContext) {
	if ec == nil {
		return
	}

	ec.Request = nil
	ec.Result = nil
	ec.Ctx = nil
	execContextPool.Put(ec)
}

func (p *Pipeline) PooledState() bool {
	if p == nil {
		return false
	}
	return p.pooled
}
