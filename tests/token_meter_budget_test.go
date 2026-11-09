//

package tests

import (
	"testing"

	"github.com/JopenChen/dsh-go/pkg/tokenmeter"
)

func TestTokenMeterAccumulate(t *testing.T) {
	m := tokenmeter.New(100000)
	m.Record(tokenmeter.TokenUsage{PromptTokens: 1000, CompletionTokens: 500})
	m.Record(tokenmeter.TokenUsage{PromptTokens: 2000, CompletionTokens: 300})

	prompt, comp, total := m.Totals()
	if prompt != 3000 || comp != 800 || total != 3800 {
		t.Fatalf("accumulation unexpected: prompt=%d comp=%d total=%d", prompt, comp, total)
	}
	if len(m.Records()) != 2 {
		t.Fatalf("should have 2 metering records, got %d", len(m.Records()))
	}
	if !m.HasBudget() {
		t.Fatal("should have HasBudget()=true when under budget")
	}
}

func TestTokenMeterBudgetDeny(t *testing.T) {
	const budget = 10000
	m := tokenmeter.New(budget)

	llmCalled := 0
	for i := 0; i < 20; i++ {

		if err := m.Check(); err != nil {
			break
		}
		llmCalled++
		m.Record(tokenmeter.TokenUsage{PromptTokens: 600, CompletionTokens: 400})
	}

	if !m.BudgetDenied() {
		t.Fatal("should budget-deny after accumulating to budget")
	}

	if llmCalled != 10 {
		t.Fatalf("should produce exactly 10 real LLM calls then deny, got %d", llmCalled)
	}
	if err := m.Check(); err == nil {
		t.Fatal("Check after exceeding budget should return budget deny error")
	}
}

func TestTokenMeterNoBudget(t *testing.T) {
	m := tokenmeter.New(0)
	for i := 0; i < 30; i++ {
		m.Record(tokenmeter.TokenUsage{PromptTokens: 1000, CompletionTokens: 1000})
	}
	if m.BudgetDenied() {
		t.Fatal("unlimited budget should not deny")
	}
	if _, _, total := m.Totals(); total != 30*2000 {
		t.Fatalf("unlimited should accumulate all, got %d", total)
	}
}

func TestTokenMeterSurfacePricing(t *testing.T) {
	p := tokenmeter.SurfacePricing{"assistant/message": 4, "tool/result": 2, "user/message": 1}
	cost := p.CostOf([]tokenmeter.SurfaceNode{
		{Kind: "assistant/message", Count: 3},
		{Kind: "tool/result", Count: 5},
		{Kind: "user/message", Count: 2},
	})
	// 3*4 + 5*2 + 2*1 = 12 + 10 + 2 = 24
	if cost != 24 {
		t.Fatalf("surface pricing estimate error: want 24, got %d", cost)
	}
}
