package tests

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/tools"
	"github.com/JopenChen/dsh-go/pkg/waterfall"
)

func makeEchoTool() *tools.Tool {
	return &tools.Tool{
		Name:        "echo",
		Description: "echo input",
		Execute: func(ctx context.Context, input map[string]any) (any, error) {
			return map[string]any{"echo": input["msg"]}, nil
		},
	}
}

func TestToolPipelinePreDeny(t *testing.T) {
	p := tools.NewPipeline().
		UsePre(func(ec *tools.ExecContext, next waterfall.NextFunc) error {
			if ec.Request.Input["danger"] == true {
				ec.Denied = true
				return nil
			}
			return next()
		}).
		WithTool(makeEchoTool())

	req := &tools.ToolCallRequest{
		CallID: brand.NewToolCallID("c1"),
		Tool:   "echo",
		Input:  map[string]any{"msg": "hello", "danger": true},
	}
	res := p.Run(context.Background(), req, nil)
	if !res.IsError {
		t.Fatal("pre deny should mark isError")
	}
	if !strings.Contains(res.Error, "denied") {
		t.Fatalf("error message should contain denied: %q", res.Error)
	}
	if res.Value != nil {
		t.Fatal("should have no result value after deny")
	}
}

func TestToolPipelinePreRewrite(t *testing.T) {
	p := tools.NewPipeline().
		UsePre(func(ec *tools.ExecContext, next waterfall.NextFunc) error {

			if v, ok := ec.Request.Input["msg"].(string); ok && v == "secret" {
				ec.Request.Input["msg"] = "masked"
			}
			return next()
		}).
		WithTool(makeEchoTool())

	req := &tools.ToolCallRequest{
		CallID: brand.NewToolCallID("c2"),
		Tool:   "echo",
		Input:  map[string]any{"msg": "secret"},
	}
	res := p.Run(context.Background(), req, nil)
	if res.IsError {
		t.Fatalf("should succeed after changing parameters: %s", res.Error)
	}
	val := res.Value.(map[string]any)
	if val["echo"] != "masked" {
		t.Fatalf("tool should receive rewritten parameters: %v", val)
	}
}

func TestToolPipelineExecuteError(t *testing.T) {
	failing := &tools.Tool{
		Name: "fail",
		Execute: func(ctx context.Context, input map[string]any) (any, error) {
			return nil, errors.New("exec failed")
		},
	}
	p := tools.NewPipeline().WithTool(failing)

	req := &tools.ToolCallRequest{CallID: brand.NewToolCallID("c3"), Tool: "fail", Input: map[string]any{}}
	res := p.Run(context.Background(), req, nil)
	if !res.IsError || !strings.Contains(res.Error, "exec failed") {
		t.Fatalf("tool error should propagate: %+v", res)
	}
}

func TestToolPipelineSignalCancel(t *testing.T) {
	p := tools.NewPipeline().
		UseExecute(func(ec *tools.ExecContext, next waterfall.NextFunc) error {

			ec.Signal = tools.SignalCancel
			return next()
		}).
		UsePost(func(ec *tools.ExecContext, next waterfall.NextFunc) error {

			if ec.Signal == tools.SignalCancel {
				ec.Denied = true
				return next()
			}
			return next()
		}).
		WithTool(makeEchoTool())

	req := &tools.ToolCallRequest{CallID: brand.NewToolCallID("c4"), Tool: "echo", Input: map[string]any{"msg": "x"}}
	res := p.Run(context.Background(), req, nil)
	if !res.IsError {
		t.Fatal("result after cancel should be isError")
	}
}

func TestToolPipelinePostMeta(t *testing.T) {
	p := tools.NewPipeline().
		UsePost(func(ec *tools.ExecContext, next waterfall.NextFunc) error {
			ec.Meta["duration_ms"] = 42
			return next()
		}).
		WithTool(makeEchoTool())

	req := &tools.ToolCallRequest{CallID: brand.NewToolCallID("c5"), Tool: "echo", Input: map[string]any{"msg": "hi"}}
	res := p.Run(context.Background(), req, nil)

	_ = res
}

func TestToolPipelinePostTruncate(t *testing.T) {
	longTool := &tools.Tool{
		Name: "long",
		Execute: func(ctx context.Context, input map[string]any) (any, error) {
			return strings.Repeat("x", 1000), nil
		},
	}
	p := tools.NewPipeline().
		UsePost(func(ec *tools.ExecContext, next waterfall.NextFunc) error {
			if s, ok := ec.Result.Value.(string); ok && len(s) > 10 {
				ec.Result.Value = s[:10] + "..."
			}
			return next()
		}).
		WithTool(longTool)

	req := &tools.ToolCallRequest{CallID: brand.NewToolCallID("c6"), Tool: "long", Input: map[string]any{}}
	res := p.Run(context.Background(), req, nil)
	if s := res.Value.(string); len(s) != 13 {
		t.Fatalf("truncated result length = %d, want 13", len(s))
	}
}

func TestToolPipelineResultPhase(t *testing.T) {
	p := tools.NewPipeline().
		UsePost(func(ec *tools.ExecContext, next waterfall.NextFunc) error {
			ec.Meta["processed"] = true
			return next()
		}).
		UseResult(func(ec *tools.ExecContext, next waterfall.NextFunc) error {
			if processed, _ := ec.Meta["processed"].(bool); processed {
				ec.Result.Value = map[string]any{"wrapped": true}
			}
			return next()
		}).
		WithTool(makeEchoTool())

	req := &tools.ToolCallRequest{CallID: brand.NewToolCallID("c7"), Tool: "echo", Input: map[string]any{"msg": "hi"}}
	res := p.Run(context.Background(), req, nil)
	val := res.Value.(map[string]any)
	if val["wrapped"] != true {
		t.Fatalf("result level should wrap result: %v", val)
	}
}

func TestToolPipelineAllPhases(t *testing.T) {
	var order []string
	record := func(name string) waterfall.Handler[tools.ExecContext] {
		return func(ec *tools.ExecContext, next waterfall.NextFunc) error {
			order = append(order, name)
			return next()
		}
	}

	p := tools.NewPipeline().
		UsePre(record("pre")).
		UseExecute(record("execute")).
		UsePost(record("post")).
		UseResult(record("result")).
		WithTool(makeEchoTool())

	req := &tools.ToolCallRequest{CallID: brand.NewToolCallID("c8"), Tool: "echo", Input: map[string]any{"msg": "hi"}}
	res := p.Run(context.Background(), req, nil)
	if res.IsError {
		t.Fatalf("should succeed: %s", res.Error)
	}
	if strings.Join(order, ",") != "pre,execute,post,result" {
		t.Fatalf("four-level order = %v", order)
	}
}
