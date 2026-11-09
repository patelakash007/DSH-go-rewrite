package tests

import (
	"testing"

	"github.com/JopenChen/dsh-go/pkg/agent"
	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/session"
)

func TestAgentCancelCauseClassify(t *testing.T) {
	causes := agent.AllCancelCauses()
	if len(causes) != 5 {
		t.Fatalf("cancellation causes should be exactly 5 types: %v", causes)
	}

	for _, cause := range causes {
		sl := session.NewSessionLog(brand.NewSessionID("cancel_" + string(cause)))

		_, _ = sl.Append(session.TurnStartData{Turn: 0})

		if err := agent.RecordCancel(sl, cause); err != nil {
			t.Fatalf("RecordCancel(%s) failed: %v", cause, err)
		}

		got, ok := agent.ExtractCancelCause(sl.Events())
		if !ok {
			t.Fatalf("cause %s should be extracted", cause)
		}
		if got != cause {
			t.Fatalf("cause extracted = %s, want %s", got, cause)
		}
	}
}

func TestAgentCancelViaAgentAPI(t *testing.T) {
	sl := session.NewSessionLog(brand.NewSessionID("cancel_api"))
	a := agent.NewAgent(brand.NewSessionID("cancel_api"), sl, nil, nil, nil)
	_, _ = sl.Append(session.TurnStartData{Turn: 0})

	a.Cancel(agent.CancelHook)
	got, ok := agent.ExtractCancelCause(sl.Events())
	if !ok || got != agent.CancelHook {
		t.Fatalf("extraction after Agent.Cancel = %s ok=%v, want hook", got, ok)
	}
}

func TestAgentCancelAbsent(t *testing.T) {
	sl := session.NewSessionLog(brand.NewSessionID("no_cancel"))
	_, _ = sl.Append(session.UserMessageData{Content: "hi"})

	if _, ok := agent.ExtractCancelCause(sl.Events()); ok {
		t.Fatal("should return false when no cancellation record")
	}
}
