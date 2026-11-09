//

//

package plan

import (
	"context"

	"github.com/JopenChen/dsh-go/pkg/session"
	"github.com/JopenChen/dsh-go/pkg/sysprompt"
)

const PlanPolicySectionName = "plan_policy"

const planPolicyText = "Currently in plan mode: output plans only, do not execute changes, until user approves."

func Enter(sl *session.SessionLog, sys *sysprompt.Assembler) error {
	if _, err := sl.Append(session.PlanModeData{Mode: "on"}); err != nil {
		return err
	}
	sys.Register(PlanPolicySectionName, sysprompt.SectionOrderPlanPolicy, planPolicyText)
	return nil
}

func Exit(sl *session.SessionLog, sys *sysprompt.Assembler) error {
	if _, err := sl.Append(session.PlanModeData{Mode: "off"}); err != nil {
		return err
	}
	sys.Unregister(PlanPolicySectionName)
	return nil
}

func IsActive(sl *session.SessionLog) bool {
	mode := session.FoldPlanMode(sl.Events())
	return mode.Present && mode.Mode == "on"
}

type ExitPlanModeResult struct {
	Approved bool

	Exited bool
}

type ExitPlanModeTool struct {
	ask func(ctx context.Context, prompt string, choices []string) (int, error)

	sl *session.SessionLog

	sys *sysprompt.Assembler
}

func NewExitPlanModeTool(sl *session.SessionLog, sys *sysprompt.Assembler) *ExitPlanModeTool {
	return &ExitPlanModeTool{sl: sl, sys: sys}
}

func (t *ExitPlanModeTool) SetAsker(f func(ctx context.Context, prompt string, choices []string) (int, error)) {
	t.ask = f
}

func (t *ExitPlanModeTool) Name() string { return "exit_plan_mode" }

func (t *ExitPlanModeTool) Description() string { return "Exit plan mode (requires user approval)" }

func (t *ExitPlanModeTool) Execute(ctx context.Context, input map[string]any) (any, error) {

	_, _ = t.sl.Append(session.PlanApprovalData{Approved: false})

	approved := resolveApproval(t, ctx)

	_, _ = t.sl.Append(session.PlanApprovalData{Approved: approved})

	if approved {

		if err := Exit(t.sl, t.sys); err != nil {
			return nil, err
		}
		return map[string]any{"exited": true, "approved": true}, nil
	}

	return map[string]any{"exited": false, "approved": false}, nil
}

func resolveApproval(t *ExitPlanModeTool, ctx context.Context) bool {
	if t.ask != nil {
		idx, err := t.ask(ctx, "Exit plan mode?", []string{"Approve exit", "Continue planning"})
		return err == nil && idx == 0
	}

	return true
}
