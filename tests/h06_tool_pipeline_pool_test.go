//

package tests

import (
	"context"
	"strings"
	"testing"

	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/tools"
	"github.com/JopenChen/dsh-go/pkg/waterfall"
)

// ============================================================================

// ============================================================================

func TestH06PooledPreDeny(t *testing.T) {
	p := tools.NewPipeline().SetPooled(true).
		UsePre(func(ec *tools.ExecContext, next waterfall.NextFunc) error {
			if ec.Request.Input["danger"] == true {
				ec.Denied = true
				return nil
			}
			return next()
		}).
		WithTool(makeEchoTool())

	req := &tools.ToolCallRequest{
		CallID: brand.NewToolCallID("hp1"),
		Tool:   "echo",
		Input:  map[string]any{"msg": "hi", "danger": true},
	}
	res := p.Run(context.Background(), req, nil)
	if !res.IsError {
		t.Fatal("pooled pre deny should mark isError")
	}
	if !strings.Contains(res.Error, "denied") {
		t.Fatalf("pooled deny error should contain denied: %q", res.Error)
	}
	if res.Value != nil {
		t.Fatal("pooled deny should have no value")
	}

	if !p.PooledState() {
		t.Fatal("PooledState should be true")
	}
}

func TestH06PooledExecuteResult(t *testing.T) {
	p := tools.NewPipeline().SetPooled(true).WithTool(makeEchoTool())
	req := &tools.ToolCallRequest{
		CallID: brand.NewToolCallID("hp2"),
		Tool:   "echo",
		Input:  map[string]any{"msg": "world"},
	}
	res := p.Run(context.Background(), req, nil)
	if res.IsError {
		t.Fatalf("pooled normal execution should not error: %s", res.Error)
	}
	v, ok := res.Value.(map[string]any)
	if !ok {
		t.Fatalf("res.Value type unexpected: %T", res.Value)
	}
	if v["echo"] != "world" {
		t.Fatalf("echo value unexpected: %v", v)
	}
}

func TestH06PooledResultIndependent(t *testing.T) {
	p := tools.NewPipeline().SetPooled(true).WithTool(makeEchoTool())
	ctx := context.Background()

	results := make([]*tools.ToolCallResult, 0, 5)
	for i := 0; i < 5; i++ {
		req := &tools.ToolCallRequest{
			CallID: brand.NewToolCallID("ind-" + itoa(i)),
			Tool:   "echo",
			Input:  map[string]any{"msg": "m" + itoa(i)},
		}
		results = append(results, p.Run(ctx, req, nil))
	}

	for i, r := range results {
		if r.CallID.Raw() != "ind-"+itoa(i) {
			t.Fatalf("result %d CallID contaminated: %s", i, r.CallID.Raw())
		}
		v, ok := r.Value.(map[string]any)
		if !ok || v["echo"] != "m"+itoa(i) {
			t.Fatalf("result %d value contaminated: %+v", i, r.Value)
		}
	}
}

func TestH06PooledMetaIsolation(t *testing.T) {

	p := tools.NewPipeline().SetPooled(true).
		UseExecute(func(ec *tools.ExecContext, next waterfall.NextFunc) error {
			if len(ec.Meta) != 0 {
				t.Fatalf("Meta leftover when reusing ExecContext: %+v", ec.Meta)
			}
			return next()
		}).
		UsePost(func(ec *tools.ExecContext, next waterfall.NextFunc) error {

			ec.Meta["tag"] = ec.Request.Input["tag"]
			return next()
		}).
		WithTool(makeEchoTool())
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		req := &tools.ToolCallRequest{
			CallID: brand.NewToolCallID("meta-" + itoa(i)),
			Tool:   "echo",
			Input:  map[string]any{"msg": "x", "tag": "t" + itoa(i)},
		}
		p.Run(ctx, req, nil)
	}
}

// ============================================================================

// ============================================================================

func BenchmarkH06RunUnPooled(b *testing.B) {
	p := tools.NewPipeline().WithTool(makeEchoTool())
	ctx := context.Background()
	req := &tools.ToolCallRequest{
		CallID: brand.NewToolCallID("bench"), Tool: "echo",
		Input: map[string]any{"msg": "x"},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = p.Run(ctx, req, nil)
	}
}

func BenchmarkH06RunPooled(b *testing.B) {
	p := tools.NewPipeline().SetPooled(true).WithTool(makeEchoTool())
	ctx := context.Background()
	req := &tools.ToolCallRequest{
		CallID: brand.NewToolCallID("bench"), Tool: "echo",
		Input: map[string]any{"msg": "x"},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = p.Run(ctx, req, nil)
	}
}
