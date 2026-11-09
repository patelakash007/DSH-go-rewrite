//

package tests

import (
	"context"
	"sync"
	"testing"

	"github.com/JopenChen/dsh-go/pkg/approval"
	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/tools"
)

func newToolCallRequest(callID, tool string, input map[string]any) *tools.ToolCallRequest {
	return &tools.ToolCallRequest{
		CallID: brand.NewToolCallID(callID),
		Tool:   tool,
		Input:  input,
	}
}

type countingAsk struct {
	sync.Mutex
	calls   int
	allowed bool
}

func (c *countingAsk) Ask(req *tools.ToolCallRequest) (bool, error) {
	c.Lock()
	defer c.Unlock()
	c.calls++
	return c.allowed, nil
}

func (c *countingAsk) count() int {
	c.Lock()
	defer c.Unlock()
	return c.calls
}

func decisionByTool(allow map[string]bool, ask map[string]bool) tools.DecisionFunc {
	return func(req *tools.ToolCallRequest) (tools.PreToolDecision, error) {
		if ask[req.Tool] {
			return tools.PreAsk, nil
		}
		if allow[req.Tool] {
			return tools.PreAllow, nil
		}
		return tools.PreDeny, nil
	}
}

func TestPreToolDecisionAllowDenyAsk(t *testing.T) {
	tool := &tools.Tool{Name: "test_tool", Execute: func(ctx context.Context, input map[string]any) (any, error) {
		return "executed", nil
	}}

	executed := false

	inner := tool.Execute
	tool.Execute = func(ctx context.Context, input map[string]any) (any, error) {
		executed = true
		return inner(ctx, input)
	}

	ask := &countingAsk{allowed: true}
	pipeline := tools.NewPipeline()
	pipeline.UsePre(tools.PreDecisionMiddleware(
		decisionByTool(map[string]bool{"allow_tool": true}, map[string]bool{"ask_tool": true}),
		ask.Ask,
	))
	pipeline.WithTool(tool)

	res := pipeline.Run(context.Background(), newToolCallRequest("c-allow", "allow_tool", nil), tool)
	if res.IsError || res.Value != "executed" {
		t.Fatalf("allow should execute successfully: %+v", res)
	}

	executed = false
	res = pipeline.Run(context.Background(), newToolCallRequest("c-deny", "deny_tool", nil), tool)
	if !res.IsError {
		t.Fatal("deny should cause isError")
	}
	if executed {
		t.Fatal("denied tool should not be executed")
	}

	executed = false
	res = pipeline.Run(context.Background(), newToolCallRequest("c-ask-1", "ask_tool", nil), tool)
	if res.IsError || res.Value != "executed" {
		t.Fatalf("ask with user approval should execute: %+v", res)
	}
	if !executed {
		t.Fatal("tool should execute after ask approval")
	}
	if ask.count() != 1 {
		t.Fatalf("should ask once, got %d", ask.count())
	}
}

func TestPreToolDecisionAskOnceNotPermanent(t *testing.T) {
	tool := &tools.Tool{Name: "t", Execute: func(ctx context.Context, input map[string]any) (any, error) {
		return "ok", nil
	}}
	ask := &countingAsk{allowed: true}
	pipeline := tools.NewPipeline()
	pipeline.UsePre(tools.PreDecisionMiddleware(
		decisionByTool(nil, map[string]bool{"ask_tool": true}),
		ask.Ask,
	))
	pipeline.WithTool(tool)

	pipeline.Run(context.Background(), newToolCallRequest("c1", "ask_tool", nil), tool)
	pipeline.Run(context.Background(), newToolCallRequest("c2", "ask_tool", nil), tool)

	if ask.count() != 2 {
		t.Fatalf("allowed-once semantic should ask on each call (2 times total), got %d", ask.count())
	}
}

func TestPreToolDecisionAskDeniedNotExecutes(t *testing.T) {
	executed := false
	tool := &tools.Tool{Name: "t", Execute: func(ctx context.Context, input map[string]any) (any, error) {
		executed = true
		return "ok", nil
	}}
	ask := &countingAsk{allowed: false}
	pipeline := tools.NewPipeline()
	pipeline.UsePre(tools.PreDecisionMiddleware(
		decisionByTool(nil, map[string]bool{"ask_tool": true}),
		ask.Ask,
	))
	pipeline.WithTool(tool)

	res := pipeline.Run(context.Background(), newToolCallRequest("c1", "ask_tool", nil), tool)
	if !res.IsError {
		t.Fatal("rejected ask should be isError")
	}
	if executed {
		t.Fatal("tool should not execute after ask rejected")
	}
}

func TestPreDecisionFromApproval(t *testing.T) {

	evaluate := func(*tools.ToolCallRequest) (approval.Decision, error) {
		return approval.DecideDeny, nil
	}
	decide := tools.DecisionFromApproval(evaluate)
	req := newToolCallRequest("c1", "bash", nil)
	dec, err := decide(req)
	if err != nil {
		t.Fatal(err)
	}
	if dec != tools.PreDeny {
		t.Fatalf("should pass-through deny, got %d", dec)
	}

	evaluateAllow := func(*tools.ToolCallRequest) (approval.Decision, error) {
		return approval.DecideAllow, nil
	}
	if ok, _ := tools.AskFromApproval(evaluateAllow)(req); !ok {
		t.Fatal("AskFromApproval should return true when allowed")
	}
}
