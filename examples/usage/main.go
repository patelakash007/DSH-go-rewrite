//

//

//   go run ./examples/usage
//

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/commands"
	"github.com/JopenChen/dsh-go/pkg/credentials"
	"github.com/JopenChen/dsh-go/pkg/goal"
	"github.com/JopenChen/dsh-go/pkg/llm"
	"github.com/JopenChen/dsh-go/pkg/llm/provider_deepseek"
	"github.com/JopenChen/dsh-go/pkg/persistence"
	"github.com/JopenChen/dsh-go/pkg/session"
)

func main() {
	ctx := context.Background()

	// ------------------------------------------------------------------

	// ------------------------------------------------------------------
	sl := session.NewSessionLog(brand.NewSessionID("demo-session-1"))

	if _, err := sl.Append(session.UserMessageData{Content: "Help me evaluate DSH-go usage"}); err != nil {
		panic(err)
	}

	if _, err := sl.Append(session.AssistantMessageData{Content: "Sure, analyzing now."}); err != nil {
		panic(err)
	}

	proj := session.FoldAll(sl.Events())
	fmt.Printf("— Events count=%d, derived messages=%d —\n", len(sl.Events()), len(proj.Messages))
	for _, m := range proj.Messages {
		fmt.Printf("  [%s] %s\n", m.Role, m.Content)
	}

	sl.EnableIncrementalProjection()
	fmt.Printf("— Incremental projection: goal.Present=%v, todo.Present=%v —\n",
		sl.Projection().Goal.Present, sl.Projection().Todo.Present)

	// ------------------------------------------------------------------

	// ------------------------------------------------------------------
	toolset := goal.NewGoalToolset(sl)

	names := make([]string, 0, len(toolset.Tools()))
	for _, t := range toolset.Tools() {
		names = append(names, t.Name)
	}
	fmt.Printf("— Goal toolset: %v —\n", names)

	var ge *goal.GoalError
	if _, err := callTool(toolset, "goal_set_max_rounds", map[string]any{"maxRounds": float64(-1)}); err != nil {
		if asErr, ok := err.(*goal.GoalError); ok {
			ge = asErr
		}
	}
	if ge != nil {
		fmt.Printf("— Stable error code: %s —\n", ge.Code)
	}

	// ------------------------------------------------------------------

	// ------------------------------------------------------------------
	cmd := commands.NewRegistry()
	out, err := cmd.Dispatch(ctx, "/plan on", sl)
	if err != nil {
		fmt.Printf("  /plan execution error: %v\n", err)
	} else {
		fmt.Printf("— Command /plan on => %s —\n", out)
	}

	// ------------------------------------------------------------------

	// ------------------------------------------------------------------
	store := credentials.NewMemoryStore()
	if err := store.Set(ctx, brand.NewCredentialRef("DEEPSEEK_API_KEY"), "sk-your-api-key"); err != nil {
		panic(err)
	}
	d := provider_deepseek.NewDeepSeek(store)
	tp := d.Transport()
	fmt.Printf("— DeepSeek connection pool: MaxIdleConnsPerHost=%d, IdleConnTimeout=%v —\n",
		tp.MaxIdleConnsPerHost, tp.IdleConnTimeout)

	//   usage, err := d.Chat(ctx, llm.ChatRequest{Model: "deepseek-chat", Messages: ...},
	//       func(c llm.StreamChunk) { _ = c })

	kind := llm.ClassifyProviderDetail("Insufficient_quota for the workspace")
	fmt.Printf("— Provider text classification => kind=%q —\n", kind)
	f := llm.NewProviderFailure("quota exceeded")
	fmt.Printf("— NewProviderFailure => kind=%q —\n", f.Kind)

	// ------------------------------------------------------------------

	// ------------------------------------------------------------------
	dir := filepath.Join(".", "tmp-dsh-demo")
	_ = os.RemoveAll(dir)
	if err := persistDemo(ctx, sl, dir); err != nil {
		fmt.Printf("  Persistence demo: %v\n", err)
	}
	_ = os.RemoveAll(dir)

	fmt.Println("\nExample completed: DSH-go integrates DeepSeek Agent planning in Go services.")
}

func callTool(ts *goal.GoalToolset, name string, input map[string]any) (any, error) {
	for _, t := range ts.Tools() {
		if t.Name == name {
			return t.Execute(context.Background(), input)
		}
	}
	return nil, fmt.Errorf("demo: tool %q not found", name)
}

func persistDemo(ctx context.Context, sl *session.SessionLog, dir string) error {
	backend, err := persistence.NewJSONL(dir,
		100,
		persistence.WithShardCount(4))
	if err != nil {
		return err
	}
	defer backend.Close()
	id := sl.SessionID()

	if err := backend.SaveHeader(ctx, session.NewSessionHeader(id, "/workspace")); err != nil {
		return err
	}
	for _, ev := range sl.Events() {
		if err := backend.Append(ctx, id, ev); err != nil {
			return err
		}
	}

	if err := backend.Flush(ctx, id); err != nil {
		return err
	}
	_, loaded, err := backend.Load(ctx, id)
	if err != nil {
		return err
	}
	fmt.Printf("— Persistence read-back: %d events —\n", len(loaded))
	return nil
}
