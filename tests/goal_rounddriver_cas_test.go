package tests

import (
	"context"
	"testing"

	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/goal"
	"github.com/JopenChen/dsh-go/pkg/session"
	"github.com/JopenChen/dsh-go/pkg/tools"
)

func TestGoalRoundDriverAutoRounds(t *testing.T) {
	sl := session.NewSessionLog(brand.NewSessionID("goal_1"))
	ts := goal.NewGoalToolset(sl)

	root, _ := ts.Tools()[0].Name, ts.Tools()[0]
	_ = root
	g := goal.NewGoal("g1", "implement task", 5)
	if err := writeGoalForTest(sl, g); err != nil {
		t.Fatalf("write goal failed: %v", err)
	}

	driver := goal.NewRoundDriver()
	rounds := 0
	for {
		if !driver.OnTurnStopping(sl, rounds) {
			break
		}
		rounds++
		if rounds > 10 {
			t.Fatal("continued rounds exceeded upper limit, suspected infinite loop")
		}
	}
	if rounds != 5 {
		t.Fatalf("maxRounds=5 should continue 5 rounds automatically, got %d", rounds)
	}
}

func TestGoalReportBlockerConcludeTurn(t *testing.T) {
	sl := session.NewSessionLog(brand.NewSessionID("goal_2"))
	ts := goal.NewGoalToolset(sl)
	if err := writeGoalForTest(sl, goal.NewGoal("g2", "task", 3)); err != nil {
		t.Fatalf("write goal failed: %v", err)
	}

	concluded := false

	ctx := tools.WithConclude(context.Background(), func(reason string) { concluded = true })
	tool := ts.Tools()[5] // goal_report_blocker
	_, err := tool.Execute(ctx, map[string]any{"blocker": "missing dependency"})
	if err != nil {
		t.Fatalf("goal_report_blocker failed: %v", err)
	}
	if !concluded {
		t.Fatal("report_blocker should trigger concludeTurn")
	}

	cur, _ := goal.FromLog(sl)
	if cur.Phase != goal.PhaseBlocked {
		t.Fatalf("phase after report_blocker should be blocked: %+v", cur)
	}
}

func TestGoalCASRevision(t *testing.T) {
	sl := session.NewSessionLog(brand.NewSessionID("goal_3"))
	ts := goal.NewGoalToolset(sl)

	if err := writeGoalForTest(sl, goal.NewGoal("g3", "task", 3)); err != nil {
		t.Fatalf("write goal failed: %v", err)
	}

	tool := ts.Tools()[2]
	if _, err := tool.Execute(context.Background(), map[string]any{"description": "new description 1"}); err != nil {
		t.Fatalf("first set_description failed: %v", err)
	}
	if _, err := tool.Execute(context.Background(), map[string]any{"description": "new description 2"}); err != nil {
		t.Fatalf("second set_description failed: %v", err)
	}
	cur, _ := goal.FromLog(sl)
	if cur.Revision != 3 {
		t.Fatalf("revision should increase monotonically to 3: %d", cur.Revision)
	}
	if cur.Description != "new description 2" {
		t.Fatalf("description should be latest: %q", cur.Description)
	}
}

func TestGoalSixTools(t *testing.T) {
	sl := session.NewSessionLog(brand.NewSessionID("goal_4"))
	ts := goal.NewGoalToolset(sl)
	if err := writeGoalForTest(sl, goal.NewGoal("g4", "task", 3)); err != nil {
		t.Fatalf("write goal failed: %v", err)
	}
	if len(ts.Tools()) != 6 {
		t.Fatalf("should have 6 goal tools: %d", len(ts.Tools()))
	}
	names := []string{}
	for _, tl := range ts.Tools() {
		names = append(names, tl.Name)
	}

	out, err := ts.Tools()[0].Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("goal_list failed: %v", err)
	}
	if out.(map[string]any)["goal"] == nil {
		t.Fatal("goal_list should read goal")
	}
	_ = names
}

func writeGoalForTest(sl *session.SessionLog, g goal.Goal) error {
	_, err := sl.Append(session.GoalChangeData{
		GoalID:      g.ID,
		Phase:       session.GoalPhase(g.Phase),
		Description: g.Description,
		MaxRounds:   g.MaxRounds,
		Revision:    g.Revision,
	})
	return err
}
