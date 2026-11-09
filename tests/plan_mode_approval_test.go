package tests

import (
	"context"
	"testing"

	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/plan"
	"github.com/JopenChen/dsh-go/pkg/session"
	"github.com/JopenChen/dsh-go/pkg/sysprompt"
)

func TestPlanModeInjectPolicy(t *testing.T) {
	sl := session.NewSessionLog(brand.NewSessionID("plan_1"))
	sys := sysprompt.New()

	if err := plan.Enter(sl, sys); err != nil {
		t.Fatalf("Enter failed: %v", err)
	}
	if !plan.IsActive(sl) {
		t.Fatal("should be in plan mode after Enter")
	}
	if !sys.Has(plan.PlanPolicySectionName) {
		t.Fatal("plan:policy section should be injected")
	}
	if !plan.IsActive(sl) {
		t.Fatal("should be in plan mode")
	}
}

func TestPlanModeApprovalPass(t *testing.T) {
	sl := session.NewSessionLog(brand.NewSessionID("plan_2"))
	sys := sysprompt.New()
	_ = plan.Enter(sl, sys)

	tool := plan.NewExitPlanModeTool(sl, sys)

	res, err := tool.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	m := res.(map[string]any)
	if m["exited"] != true || m["approved"] != true {
		t.Fatalf("approval should exit: %v", m)
	}

	if sys.Has(plan.PlanPolicySectionName) {
		t.Fatal("plan:policy should be removed after approval")
	}
	if plan.IsActive(sl) {
		t.Fatal("should exit plan mode after approval")
	}
}

func TestPlanModeApprovalReject(t *testing.T) {
	sl := session.NewSessionLog(brand.NewSessionID("plan_3"))
	sys := sysprompt.New()
	_ = plan.Enter(sl, sys)

	tool := plan.NewExitPlanModeTool(sl, sys)

	tool.SetAsker(func(ctx context.Context, prompt string, choices []string) (int, error) {
		return 1, nil
	})
	res, err := tool.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	m := res.(map[string]any)
	if m["exited"] != false || m["approved"] != false {
		t.Fatalf("denial should not exit: %v", m)
	}

	if !sys.Has(plan.PlanPolicySectionName) {
		t.Fatal("plan:policy should be retained after denial")
	}
	if !plan.IsActive(sl) {
		t.Fatal("should still be in plan mode after denial")
	}
}
