//

package tests

import (
	"strings"
	"testing"
	"time"

	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/session"
)

func buildHealthyLog(t *testing.T) []session.SessionEvent {
	sl := session.NewSessionLog(brand.NewSessionID("n02"))
	_, _ = sl.Append(session.TurnStartData{Turn: 0})
	_, _ = sl.Append(session.StepStartData{Turn: 0, Step: 1})
	aid := brand.NewApprovalRequestID("ap1")
	_, _ = sl.Append(session.ApprovalRequestIDData{RequestID: aid, Tool: "bash"})
	_, _ = sl.Append(session.ApprovalDecidedData{RequestID: aid, Allowed: true})
	callID := brand.NewToolCallID("tc1")
	_, _ = sl.Append(session.ToolCallData{CallID: callID, Tool: "echo"})
	_, _ = sl.Append(session.ToolResultData{CallID: callID, Output: "hi"})
	_, _ = sl.Append(session.GoalChangeData{GoalID: "g1", Phase: session.GoalPhase("active"), Revision: 1})
	_, _ = sl.Append(session.GoalChangeData{GoalID: "g1", Revision: 2})
	_, _ = sl.Append(session.StepEndData{Turn: 0, Step: 1})
	_, _ = sl.Append(session.TurnEndData{Turn: 0, Reason: session.ReasonFinished})
	return sl.Events()
}

func TestN02AllInvariantsPass(t *testing.T) {
	events := buildHealthyLog(t)
	if fails := session.VerifyInvariants(events); len(fails) != 0 {
		t.Fatalf("healthy log should have no invariant failures: %v", fails)
	}
}

func TestN02InvSeqGapCaptured(t *testing.T) {
	events := buildHealthyLog(t)

	corrupt := append([]session.SessionEvent{}, events...)
	corrupt[2].Seq = 999
	corrupt[2].Type = session.EventUserMessage
	fails := session.VerifyInvariants(corrupt)
	if !hasInvariant(fails, session.InvSeqContinuous) {
		t.Fatalf("skipped seq should be caught by seq_continuous: %v", fails)
	}
}

func TestN02InvTimeRewindCaptured(t *testing.T) {
	events := buildHealthyLog(t)
	corrupt := append([]session.SessionEvent{}, events...)
	corrupt[3].Time = corrupt[2].Time.Add(-time.Hour)
	fails := session.VerifyInvariants(corrupt)
	if !hasInvariant(fails, session.InvTimeMonotonic) {
		t.Fatalf("time rollback should be caught by time_monotonic: %v", fails)
	}
}

func TestN02InvUnpairedTurnCaptured(t *testing.T) {

	var events []session.SessionEvent
	ev := session.SessionEvent{Seq: 1, Time: time.Now(), Type: session.EventTurnStart, Data: session.TurnStartData{Turn: 0}}
	events = append(events, ev)
	fails := session.VerifyInvariants(events)
	if !hasInvariant(fails, session.InvTurnPaired) {
		t.Fatalf("unpaired turn should be caught: %v", fails)
	}
}

func TestN02InvGoalCASCaptured(t *testing.T) {
	events := buildHealthyLog(t)

	extra := session.SessionEvent{Seq: 100, Time: time.Now().Add(time.Minute), Type: session.EventGoalChange,
		Data: session.GoalChangeData{GoalID: "g1", Revision: 4}}
	events = append(events, extra)
	fails := session.VerifyInvariants(events)
	if !hasInvariant(fails, session.InvGoalCAS) {
		t.Fatalf("skipped goal revision should be caught by goal_revision_cas: %v", fails)
	}
}

func TestN02InvUnknownTypeCaptured(t *testing.T) {
	events := buildHealthyLog(t)
	corrupt := append([]session.SessionEvent{}, events...)
	corrupt[0].Type = session.EventType("bogus/type")
	fails := session.VerifyInvariants(corrupt)
	if !hasInvariant(fails, session.InvFormatConsistent) {
		t.Fatalf("unknown type should be caught by persistence_format_consistent: %v", fails)
	}
}

func TestN02FiftyRoundsSeqTime(t *testing.T) {
	sl := session.NewSessionLog(brand.NewSessionID("n02-50"))
	for i := uint64(0); i < 50; i++ {
		_, _ = sl.Append(session.TurnStartData{Turn: i})
		_, _ = sl.Append(session.StepStartData{Turn: i, Step: 1})
		_, _ = sl.Append(session.UserMessageData{Content: "hi"})
		_, _ = sl.Append(session.AssistantMessageData{Content: "ok"})
		_, _ = sl.Append(session.StepEndData{Turn: i, Step: 1})
		_, _ = sl.Append(session.TurnEndData{Turn: i, Reason: session.ReasonFinished})
	}
	events := sl.Events()
	if fails := session.VerifyInvariants(events); len(fails) != 0 {
		t.Fatalf("invariants should pass after 50 rounds: %v", fails)
	}

	if len(events) != 300 || events[299].Seq != 300 {
		t.Fatalf("should have 300 events with final seq=300, got %d/%d", len(events), events[len(events)-1].Seq)
	}

	for i := 1; i < len(events); i++ {
		if events[i].Time.Before(events[i-1].Time) {
			t.Fatalf("time non-monotonic at %d", i)
		}
	}
}

func TestN02AppendOnlyContract(t *testing.T) {
	sl := session.NewSessionLog(brand.NewSessionID("ao"))
	_, _ = sl.Append(session.UserMessageData{Content: "a"})
	_, _ = sl.Append(session.UserMessageData{Content: "b"})

	events := sl.Events()
	events[0].Data = session.UserMessageData{Content: "HACK"}
	if sl.Len() != 2 {
		t.Fatal("read-only copy operation should not alter Len")
	}
	inner := sl.Events()
	if um, ok := inner[0].Data.(session.UserMessageData); !ok || um.Content != "a" {
		t.Fatal("mutating copy in-place should not affect internal events (strictly append-only)")
	}
}

func hasInvariant(fails []error, code session.InvariantCode) bool {
	for _, f := range fails {
		if strings.Contains(f.Error(), "["+string(code)+"]") {
			return true
		}
	}
	return false
}
