//

//

//

//   go run ./examples/tutorial
//

package main

import (
	"context"
	"fmt"

	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/goal"
	"github.com/JopenChen/dsh-go/pkg/session"
)

func main() {

	step1EventSourcing()
	fmt.Println()
	step2FoldProjection()
	fmt.Println()
	step3GoalStateMachine()
}

// ---------------------------------------------------------------------------

//

//

// ---------------------------------------------------------------------------
func step1EventSourcing() {
	fmt.Println("=== Step 1: Event Sourcing (append-only event log) ===")

	sl := session.NewSessionLog(brand.NewSessionID("tutorial-1"))

	_, _ = sl.Append(session.UserMessageData{Content: "Help me evaluate DSH-go"})
	_, _ = sl.Append(session.AssistantMessageData{Content: "Sure, let us begin."})

	evs := sl.Events()
	fmt.Printf("— Appended events count: %d —\n", len(evs))
	for i, ev := range evs {
		fmt.Printf("  #%d seq=%d type=%s\n", i+1, ev.Seq, ev.Type)
	}

	fmt.Println("— Conclusion: State = cumulative result of event log —")
}

// ---------------------------------------------------------------------------

//

//

// ---------------------------------------------------------------------------
func step2FoldProjection() {
	fmt.Println("=== Step 2: fold Derived Projections ===")

	sl := session.NewSessionLog(brand.NewSessionID("tutorial-2"))
	_, _ = sl.Append(session.UserMessageData{Content: "Hello"})
	_, _ = sl.Append(session.AssistantMessageData{Content: "Hello, I am DSH-go."})

	proj := session.FoldAll(sl.Events())
	fmt.Printf("— Derived messages count: %d —\n", len(proj.Messages))
	for _, m := range proj.Messages {
		fmt.Printf("  [%s] %s\n", m.Role, m.Content)
	}

	if proj.PlanMode.Present {
		fmt.Printf("— Derived plan mode: %v —\n", proj.PlanMode.Mode)
	}
	fmt.Println("— Conclusion: Projection = f(event log) —")
}

// ---------------------------------------------------------------------------

//

//

//

// ---------------------------------------------------------------------------
func step3GoalStateMachine() {
	fmt.Println("=== Step 3: Goal Planning State Machine ===")

	sl := session.NewSessionLog(brand.NewSessionID("tutorial-3"))
	ts := goal.NewGoalToolset(sl)

	var names []string
	for _, t := range ts.Tools() {
		names = append(names, t.Name)
	}
	fmt.Printf("— Goal toolset: %v —\n", names)

	if _, err := call(ts, "goal_set_description", map[string]any{"description": "Complete evaluation report"}); err != nil {
		fmt.Printf("  set_description err: %v\n", err)
	}
	if _, err := call(ts, "goal_set_max_rounds", map[string]any{"maxRounds": float64(5)}); err != nil {
		fmt.Printf("  set_max_rounds err: %v\n", err)
	}

	st := sl.Projection().Goal
	fmt.Printf("— Derived goal state: present=%v, phase=%q, maxRounds=%d —\n",
		st.Present, st.Phase, st.MaxRounds)

	if _, err := call(ts, "goal_set_max_rounds", map[string]any{"maxRounds": float64(-1)}); err != nil {
		if ge, ok := err.(*goal.GoalError); ok {
			fmt.Printf("— Stable error code example: %s —\n", ge.Code)
		} else {
			fmt.Printf("— Other error: %v —\n", err)
		}
	}
	fmt.Println("— Conclusion: Planning = State machine —")
}

func call(ts *goal.GoalToolset, name string, input map[string]any) (any, error) {
	for _, t := range ts.Tools() {
		if t.Name == name {
			return t.Execute(context.Background(), input)
		}
	}
	return nil, fmt.Errorf("demo: tool %q not found", name)
}
