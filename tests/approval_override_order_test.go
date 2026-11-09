//

package tests

import (
	"context"
	"testing"

	"github.com/JopenChen/dsh-go/pkg/approval"
	"github.com/JopenChen/dsh-go/pkg/userq"
)

type fixedAnswerProvider struct{ ans int }

func (f *fixedAnswerProvider) Ask(_ context.Context, _ userq.QuestionOptions) (*userq.QuestionResult, error) {
	return &userq.QuestionResult{SelectedIndex: f.ans, SelectedIndices: []int{f.ans}}, nil
}

type failingAnswerProvider struct{}

func (*failingAnswerProvider) Ask(context.Context, userq.QuestionOptions) (*userq.QuestionResult, error) {
	return nil, context.Canceled
}

func presetPolicy(preset string) (approval.Policy, bool) {
	switch preset {
	case "safe":
		return approval.PolicyDenyAll, true
	case "danger":
		return approval.PolicyAllowAll, true
	case "default":
		return approval.PolicyAskDangerous, true
	}
	return approval.PolicyDenyAll, false
}

func TestApprovalThreeLayerOverrideOrder(t *testing.T) {
	svc := approval.New(presetPolicy, userq.New(userq.NewStub()))

	if e := svc.Resolve("danger", ""); e.Source != approval.SourcePreset || e.Policy != approval.PolicyAllowAll {
		t.Fatalf("preset layer error: %+v", e)
	}

	svc.SetUserPolicy(approval.PolicyDenyAll)
	if e := svc.Resolve("danger", ""); e.Source != approval.SourceUser || e.Policy != approval.PolicyDenyAll {
		t.Fatalf("user layer override error: %+v", e)
	}

	svc.SetSessionPolicy("sess1", approval.PolicyAllowAll)
	if e := svc.Resolve("danger", "sess1"); e.Source != approval.SourceSession || e.Policy != approval.PolicyAllowAll {
		t.Fatalf("session layer override error: %+v", e)
	}

	if e := svc.Resolve("danger", "sess-other"); e.Source != approval.SourceUser {
		t.Fatalf("other sessions should match user layer: %+v", e)
	}
}

func TestApprovalEvaluateAllowDeny(t *testing.T) {
	svc := approval.New(presetPolicy, userq.New(userq.NewStub()))
	dec, _ := svc.Evaluate(approval.Request{Tool: "bash", Preset: "danger", CallID: "c1"})
	if dec != approval.DecideAllow {
		t.Fatalf("danger(allow-all) should allow, got %s", dec)
	}
	dec, _ = svc.Evaluate(approval.Request{Tool: "bash", Preset: "safe", CallID: "c2"})
	if dec != approval.DecideDeny {
		t.Fatalf("safe(deny-all) should deny, got %s", dec)
	}
}

func TestApprovalAskAllowedOnce(t *testing.T) {

	svc := approval.New(presetPolicy, userq.New(&fixedAnswerProvider{ans: 0}))

	dec, err := svc.Evaluate(approval.Request{Tool: "bash", Preset: "default", CallID: "call-1"})
	if err != nil {
		t.Fatal(err)
	}
	if dec != approval.DecideAllow {
		t.Fatalf("ask with user approval should allow, got %s", dec)
	}
	if svc.AskCount() != 1 {
		t.Fatalf("should ask exactly once, got %d", svc.AskCount())
	}

	dec, err = svc.Evaluate(approval.Request{Tool: "bash", Preset: "default", CallID: "call-2"})
	if err != nil {
		t.Fatal(err)
	}
	if svc.AskCount() != 2 {
		t.Fatalf("allowed-once semantic should ask again for next tool call, got %d", svc.AskCount())
	}
	if dec != approval.DecideAllow {
		t.Fatalf("second call with user approval should allow, got %s", dec)
	}
}

func TestApprovalAskRejected(t *testing.T) {

	svc := approval.New(presetPolicy, userq.New(&fixedAnswerProvider{ans: 1}))
	dec, err := svc.Evaluate(approval.Request{Tool: "bash", Preset: "default", CallID: "c1"})
	if err != nil {
		t.Fatal(err)
	}
	if dec != approval.DecideDeny {
		t.Fatalf("ask rejected should deny, got %s", dec)
	}
}

func TestApprovalUQFailureDeny(t *testing.T) {
	svc := approval.New(presetPolicy, userq.New(&failingAnswerProvider{}))
	dec, err := svc.Evaluate(approval.Request{Tool: "bash", Preset: "default", CallID: "c1"})
	if err == nil {
		t.Fatal("UQ failure should bubble error")
	}
	if dec != approval.DecideDeny {
		t.Fatalf("UQ failure should fail-closed deny, got %s", dec)
	}
}

func TestApprovalNonDangerousAllows(t *testing.T) {
	svc := approval.New(presetPolicy, userq.New(&fixedAnswerProvider{ans: 1}))
	dec, err := svc.Evaluate(approval.Request{Tool: "goal_list", Preset: "default", CallID: "c1"})
	if err != nil {
		t.Fatal(err)
	}
	if dec != approval.DecideAllow {
		t.Fatalf("non-dangerous tool should allow directly, got %s", dec)
	}
	if svc.AskCount() != 0 {
		t.Fatalf("non-dangerous tool should not ask, got %d", svc.AskCount())
	}
}
