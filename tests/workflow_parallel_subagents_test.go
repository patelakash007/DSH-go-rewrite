//

package tests

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/JopenChen/dsh-go/pkg/workflow"
)

func spawnStub(ctx context.Context, backend, input string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return "agent:" + backend + ":" + input + ":done", nil
}

func TestWorkflowParallel3Subagents(t *testing.T) {

	subA := workflow.Pipeline("subA",
		workflow.Agent("a1", "in-process", "A-1", spawnStub),
		workflow.Agent("a2", "in-process", "A-2", spawnStub),
	)
	subB := workflow.Pipeline("subB",
		workflow.Agent("b1", "in-process", "B-1", spawnStub),
		workflow.Agent("b2", "in-process", "B-2", spawnStub),
	)
	subC := workflow.Pipeline("subC",
		workflow.Agent("c1", "in-process", "C-1", spawnStub),
		workflow.Agent("c2", "in-process", "C-2", spawnStub),
	)

	meta := workflow.Parallel("meta", workflow.Seq(subA), workflow.Seq(subB), workflow.Seq(subC))

	res, err := meta.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Cancelled {
		t.Fatal("normal run should not be marked cancelled")
	}
	if len(res.Outputs) != 3 {
		t.Fatalf("should have 3 subagent outputs, got %d", len(res.Outputs))
	}

	full := strings.Join(res.Outputs, ";")
	if !strings.Contains(full, "A-1:done") || !strings.Contains(full, "A-2:done") ||
		!strings.Contains(full, "B-1:done") || !strings.Contains(full, "B-2:done") ||
		!strings.Contains(full, "C-1:done") || !strings.Contains(full, "C-2:done") {
		t.Fatalf("summary should contain all 6 step results, got %q", full)
	}
}

func TestWorkflowPipelineShortCircuit(t *testing.T) {
	runCount := 0
	bad := workflow.Pipeline("bad",
		&workflow.Step{Name: "ok", Run: func(ctx context.Context) (string, error) { return "ok", nil }},
		&workflow.Step{Name: "boom", Run: func(ctx context.Context) (string, error) { return "", errBoom{} }},
		&workflow.Step{Name: "never", Run: func(ctx context.Context) (string, error) { runCount++; return "no", nil }},
	)
	res, err := bad.Run(context.Background())
	if err == nil {
		t.Fatal("serial failure should return error")
	}
	if runCount != 0 {
		t.Fatalf("subsequent steps should not execute after short-circuit, executed %d times", runCount)
	}

	if len(res.Outputs) != 2 {
		t.Fatalf("output should be 2 items (ok + error), got %d", len(res.Outputs))
	}
}

func TestWorkflowCancel(t *testing.T) {
	step := &workflow.Step{
		Name: "blocking",
		Run: func(ctx context.Context) (string, error) {

			<-ctx.Done()
			return "", ctx.Err()
		},
	}
	w := workflow.Parallel("cancel", step, step)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	res, _ := w.Run(ctx)
	if !res.Cancelled {
		t.Fatal("Result should be marked Cancelled after cancellation")
	}
}

type errBoom struct{}

func (errBoom) Error() string { return "boom" }
