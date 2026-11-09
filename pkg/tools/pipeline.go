//

//

//

package tools

import (
	"context"

	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/waterfall"
)

type Signal string

const (
	SignalContinue Signal = "continue"

	SignalCancel Signal = "cancel"
)

type ToolExecuteFunc func(ctx context.Context, input map[string]any) (any, error)

type Tool struct {
	Name string

	Description string

	Schema *JsonSchemaNode

	TimeoutMs int

	Execute ToolExecuteFunc
}

type ToolCallRequest struct {
	CallID brand.ToolCallID

	Tool string

	Input map[string]any
}

type ToolCallResult struct {
	CallID brand.ToolCallID

	IsError bool

	Value any

	Error string

	ErrorCode string
}

type ExecContext struct {
	Request *ToolCallRequest

	Result *ToolCallResult

	Signal Signal

	Denied bool

	Meta map[string]any

	Ctx context.Context
}

type Pipeline struct {
	pre     *waterfall.Chain[ExecContext]
	execute *waterfall.Chain[ExecContext]
	post    *waterfall.Chain[ExecContext]
	result  *waterfall.Chain[ExecContext]

	pooled bool
}

func (p *Pipeline) SetPooled(enabled bool) *Pipeline {
	p.pooled = enabled
	return p
}

func NewPipeline() *Pipeline {
	return &Pipeline{
		pre:     waterfall.New[ExecContext](),
		execute: waterfall.New[ExecContext](),
		post:    waterfall.New[ExecContext](),
		result:  waterfall.New[ExecContext](),
	}
}

func (p *Pipeline) UsePre(h ...waterfall.Handler[ExecContext]) *Pipeline {
	p.pre.Use(h...)
	return p
}

func (p *Pipeline) UseExecute(h ...waterfall.Handler[ExecContext]) *Pipeline {
	p.execute.Use(h...)
	return p
}

func (p *Pipeline) UsePost(h ...waterfall.Handler[ExecContext]) *Pipeline {
	p.post.Use(h...)
	return p
}

func (p *Pipeline) UseResult(h ...waterfall.Handler[ExecContext]) *Pipeline {
	p.result.Use(h...)
	return p
}

func (p *Pipeline) Run(ctx context.Context, req *ToolCallRequest, tool *Tool) *ToolCallResult {
	if p.pooled {
		return p.RunPooled(ctx, req, tool)
	}
	ec := &ExecContext{
		Request: req,
		Result:  &ToolCallResult{CallID: req.CallID},
		Signal:  SignalContinue,
		Denied:  false,
		Meta:    map[string]any{},
		Ctx:     ctx,
	}

	_ = p.pre.Run(ctx, ec)
	if ec.Denied {
		ec.Result.IsError = true
		ec.Result.Error = "denied by pre-execute middleware"
		return ec.Result
	}

	if ctx.Err() != nil {
		return AbortedBeforeDispatchResult(req.CallID)
	}
	_ = p.execute.Run(ctx, ec)
	if ec.Signal == SignalCancel {

		ec.Result.IsError = true
	}

	_ = p.post.Run(ctx, ec)
	if ec.Denied {
		ec.Result.IsError = true
		if ec.Result.Error == "" {
			ec.Result.Error = "blocked by post-execute middleware"
		}
	}

	_ = p.result.Run(ctx, ec)
	return ec.Result
}

func executeInner(tool *Tool) waterfall.Handler[ExecContext] {
	return func(ec *ExecContext, next waterfall.NextFunc) error {
		if tool == nil || tool.Execute == nil {
			ec.Result.IsError = true
			ec.Result.Error = "tool implementation missing"
			return next()
		}

		val, err := tool.Execute(withExecContext(ec.Ctx, ec), ec.Request.Input)
		if err != nil {
			ec.Result.IsError = true
			ec.Result.Error = err.Error()
		} else {
			ec.Result.Value = val
		}
		return next()
	}
}

func (p *Pipeline) WithTool(tool *Tool) *Pipeline {
	p.execute.Use(executeInner(tool))
	return p
}

func contextWithMeta(ec *ExecContext) context.Context {
	return withExecContext(context.Background(), ec)
}

type execContextKey struct{}

func withExecContext(ctx context.Context, ec *ExecContext) context.Context {
	return context.WithValue(ctx, execContextKey{}, ec)
}

func ExecContextFrom(ctx context.Context) (*ExecContext, bool) {
	ec, ok := ctx.Value(execContextKey{}).(*ExecContext)
	return ec, ok
}
