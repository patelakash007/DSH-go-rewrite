//

//

package tests

import (
	"context"
	"errors"
	"testing"

	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/goal"
	"github.com/JopenChen/dsh-go/pkg/session"
	"github.com/JopenChen/dsh-go/pkg/tools"
)

func newGoalLog(t *testing.T) *session.SessionLog {
	t.Helper()
	sl := session.NewSessionLog(brand.NewSessionID("r06-goal"))
	if _, err := sl.Append(session.GoalChangeData{
		GoalID: "g1", Phase: session.GoalPhase("active"), Description: "d", MaxRounds: 5, Revision: 1,
	}); err != nil {
		t.Fatal(err)
	}
	return sl
}

func pickTool(t *testing.T, sl *session.SessionLog, name string) *tools.Tool {
	t.Helper()
	for _, to := range goal.NewGoalToolset(sl).Tools() {
		if to.Name == name {
			return to
		}
	}
	t.Fatalf("tool %q not found", name)
	return nil
}

func TestR06PhaseHasPaused(t *testing.T) {
	if string(goal.PhasePaused) != "paused" {
		t.Fatalf("PhasePaused = %q, want paused", goal.PhasePaused)
	}
	valid := map[goal.Phase]bool{
		goal.PhaseActive: true, goal.PhasePaused: true,
		goal.PhaseBlocked: true, goal.PhaseComplete: true,
		goal.Phase("bogus"): false, goal.Phase("in-progress"): false,
	}
	for p, want := range valid {
		if p.Valid() != want {
			t.Fatalf("Valid(%q) = %v, want %v", p, p.Valid(), want)
		}
	}
}

func TestR06GoalErrorCodeAndChain(t *testing.T) {
	cause := errors.New("underlying")
	ge := goal.NewGoalError(goal.ErrorStaleRevision, "revision mismatch", cause)
	if string(ge.Code) != "GOAL_STALE_REVISION" {
		t.Fatalf("Code = %q", ge.Code)
	}
	if !errors.Is(ge, cause) {
		t.Fatal("errors.Is should traverse Cause down to underlying")
	}
	var as *goal.GoalError
	if !errors.As(ge, &as) || as.Code != goal.ErrorStaleRevision {
		t.Fatalf("errors.As should restore GoalError, got %+v", as)
	}
	if goal.FromError(ge) != ge {
		t.Fatal("FromError should return GoalError as-is")
	}
	if goal.FromError(errors.New("x")).Code != "unknown" {
		t.Fatal("FromError on standard error should classify as unknown")
	}
}

func TestR06ToolValidationCodes(t *testing.T) {
	cases := []struct {
		tool  string
		input map[string]any
		code  goal.ErrorCode
	}{
		{"goal_set_phase", map[string]any{"phase": "totally-bogus"}, goal.ErrorInvalidTransition},
		{"goal_set_max_rounds", map[string]any{"maxRounds": float64(-3)}, goal.ErrorInvalidMaxRounds},
		{"goal_add_blocker", map[string]any{"blocker": ""}, goal.ErrorInvalidBlockReason},
		{"goal_report_blocker", map[string]any{"blocker": ""}, goal.ErrorInvalidBlockReason},
	}
	for _, c := range cases {
		sl := newGoalLog(t)
		fn := pickTool(t, sl, c.tool)
		_, err := fn.Execute(context.Background(), c.input)
		var ge *goal.GoalError
		if !errors.As(err, &ge) || ge.Code != c.code {
			t.Fatalf("%s(%v) err=%v, should contain code=%q", c.tool, c.input, err, c.code)
		}
	}
}

func TestR06ReportBlockerWritesBlockReason(t *testing.T) {
	sl := newGoalLog(t)
	fn := pickTool(t, sl, "goal_report_blocker")
	_, err := fn.Execute(context.Background(), map[string]any{"blocker": "missing api key"})
	if err != nil {
		t.Fatal(err)
	}

	g, ok := goal.FromLog(sl)
	if !ok {
		t.Fatal("after report_blocker, goal should still exist")
	}
	if g.Phase != goal.PhaseBlocked {
		t.Fatalf("persisted phase = %q, want blocked", g.Phase)
	}
	if g.BlockReason == nil || g.BlockReason.Message != "missing api key" || g.BlockReason.Code == "" {
		t.Fatalf("BlockReason after persistence should be read back from log, got %+v", g.BlockReason)
	}
}

func TestR06BlockReasonValidation(t *testing.T) {
	reason, ok := goal.NewBlockReason("missing-credential", "api key absent")
	if !ok || reason.Code != "missing-credential" || reason.Message == "" {
		t.Fatalf("NewBlockReason = %+v ok=%v", reason, ok)
	}
	if _, emptyOk := goal.NewBlockReason("", "x"); emptyOk {
		t.Fatal("empty code should be judged invalid")
	}
	if _, emptyOk2 := goal.NewBlockReason("code", ""); emptyOk2 {
		t.Fatal("empty message should be judged invalid")
	}
}
