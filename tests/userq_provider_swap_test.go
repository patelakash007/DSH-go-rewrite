package tests

import (
	"context"
	"testing"

	"github.com/JopenChen/dsh-go/pkg/userq"
)

type mockProvider struct {
	idx      int
	custom   string
	callOpts userq.QuestionOptions
}

func (m *mockProvider) Ask(ctx context.Context, opts userq.QuestionOptions) (*userq.QuestionResult, error) {
	m.callOpts = opts
	return &userq.QuestionResult{SelectedIndex: m.idx, Custom: m.custom, SelectedIndices: []int{m.idx}}, nil
}

func TestUserQStubDefault(t *testing.T) {
	svc := userq.New(userq.NewStub())
	ctx := context.Background()

	idx, custom, err := svc.Ask(ctx, userq.QuestionOptions{
		Prompt:  "Continue?",
		Choices: []string{"Yes", "No"},
		Intent:  "plan-exit",
	})
	if err != nil {
		t.Fatalf("Ask failed: %v", err)
	}
	if idx != 0 {
		t.Fatalf("stub default option idx = %d, want 0", idx)
	}
	if custom != "" {
		t.Fatalf("stub custom should be empty: %q", custom)
	}
}

func TestUserQProviderSwap(t *testing.T) {
	svc := userq.New(userq.NewStub())

	mp := &mockProvider{idx: 2, custom: "Custom input"}
	svc.SetProvider(mp)

	opts := userq.QuestionOptions{Prompt: "Select", Choices: []string{"a", "b", "c"}, Intent: "approval"}
	idx, custom, err := svc.Ask(context.Background(), opts)
	if err != nil {
		t.Fatalf("Ask failed: %v", err)
	}
	if idx != 2 || custom != "Custom input" {
		t.Fatalf("should route to custom implementation: idx=%d custom=%q", idx, custom)
	}

	if mp.callOpts.Intent != "approval" || mp.callOpts.MultiSelect {
		t.Fatalf("intent pass-through unexpected: %+v", mp.callOpts)
	}
}

func TestUserQMultiSelect(t *testing.T) {
	svc := userq.New(userq.NewStub())
	ctx := context.Background()

	res, err := svc.AskDetailed(ctx, userq.QuestionOptions{
		Prompt:      "Multi-select",
		Choices:     []string{"a", "b", "c"},
		MultiSelect: true,
	})
	if err != nil {
		t.Fatalf("AskDetailed failed: %v", err)
	}
	if len(res.SelectedIndices) != 3 {
		t.Fatalf("multi-select should return all indices: %v", res.SelectedIndices)
	}
}

func TestUserQNoProvider(t *testing.T) {
	svc := userq.New(nil)
	if _, _, err := svc.Ask(context.Background(), userq.QuestionOptions{}); err == nil {
		t.Fatal("should error without provider")
	}
}
