package tests

import (
	"testing"

	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/session"
)

func appendTurn(sl *session.SessionLog, turnIdx uint64) error {
	if _, err := sl.Append(session.TurnStartData{Turn: turnIdx}); err != nil {
		return err
	}
	if _, err := sl.Append(session.StepStartData{Turn: turnIdx, Step: 1}); err != nil {
		return err
	}
	if _, err := sl.Append(session.UserMessageData{Content: "hi"}); err != nil {
		return err
	}
	if _, err := sl.Append(session.StepEndData{Turn: turnIdx, Step: 1}); err != nil {
		return err
	}
	_, err := sl.Append(session.TurnEndData{Turn: turnIdx, Reason: session.ReasonFinished})
	return err
}

func TestSessionAppendSeqContinuous(t *testing.T) {
	sl := session.NewSessionLog(brand.NewSessionID("s1"))

	seqs := []uint64{}
	for i := 0; i < 5; i++ {
		seq, err := sl.Append(session.UserMessageData{Content: "msg"})
		if err != nil {
			t.Fatalf("Append failed: %v", err)
		}
		seqs = append(seqs, seq)
	}
	for i, seq := range seqs {
		if seq != uint64(i+1) {
			t.Fatalf("seq should be contiguous 1..N, item %d = %d", i+1, seq)
		}
	}
	if sl.LastSeq() != 5 || sl.Len() != 5 {
		t.Fatalf("LastSeq=%d Len=%d, want 5/5", sl.LastSeq(), sl.Len())
	}
}

func TestSessionTurnPairing(t *testing.T) {
	sl := session.NewSessionLog(brand.NewSessionID("s1"))

	if _, err := sl.Append(session.TurnEndData{Turn: 0, Reason: session.ReasonFinished}); err == nil {
		t.Fatal("turn/end without turn/start should be rejected")
	}

	if err := appendTurn(sl, 0); err != nil {
		t.Fatalf("valid turn should pass: %v", err)
	}
}

func TestSessionStepPairing(t *testing.T) {
	sl := session.NewSessionLog(brand.NewSessionID("s1"))

	if _, err := sl.Append(session.StepStartData{Turn: 0, Step: 1}); err == nil {
		t.Fatal("step/start without open turn should be rejected")
	}

	_, _ = sl.Append(session.TurnStartData{Turn: 0})
	if _, err := sl.Append(session.StepStartData{Turn: 0, Step: 1}); err != nil {
		t.Fatalf("step/start within turn should pass: %v", err)
	}

	if _, err := sl.Append(session.StepStartData{Turn: 0, Step: 2}); err == nil {
		t.Fatal("step/start when step unclosed should be rejected")
	}
}

func TestSessionToolCallResultPairing(t *testing.T) {
	sl := session.NewSessionLog(brand.NewSessionID("s1"))
	_, _ = sl.Append(session.TurnStartData{Turn: 0})
	_, _ = sl.Append(session.StepStartData{Turn: 0, Step: 1})

	if _, err := sl.Append(session.ToolResultData{CallID: brand.NewToolCallID("ghost")}); err == nil {
		t.Fatal("tool/result without matching tool/call should be rejected")
	}

	callID := brand.NewToolCallID("call_x")
	if _, err := sl.Append(session.ToolCallData{CallID: callID, Tool: "bash"}); err != nil {
		t.Fatalf("tool/call should pass: %v", err)
	}

	if _, err := sl.Append(session.ToolCallData{CallID: callID, Tool: "bash"}); err == nil {
		t.Fatal("duplicate tool/call should be rejected")
	}

	if _, err := sl.Append(session.ToolResultData{CallID: callID, Output: "ok"}); err != nil {
		t.Fatalf("paired tool/result should pass: %v", err)
	}

	if _, err := sl.Append(session.ToolResultData{CallID: callID}); err == nil {
		t.Fatal("duplicate tool/result should be rejected")
	}
}

func TestSessionValidTurnWithTools(t *testing.T) {
	sl := session.NewSessionLog(brand.NewSessionID("s1"))

	_, _ = sl.Append(session.TurnStartData{Turn: 0})
	_, _ = sl.Append(session.StepStartData{Turn: 0, Step: 1})
	_, _ = sl.Append(session.UserMessageData{Content: "list files"})
	callID := brand.NewToolCallID("call_ls")
	_, _ = sl.Append(session.ToolCallData{CallID: callID, Tool: "bash"})
	_, _ = sl.Append(session.ToolResultData{CallID: callID, Output: "a.txt"})
	_, _ = sl.Append(session.StepEndData{Turn: 0, Step: 1})
	_, err := sl.Append(session.TurnEndData{Turn: 0, Reason: session.ReasonFinished})
	if err != nil {
		t.Fatalf("complete valid turn should pass: %v", err)
	}

	events := sl.Events()
	if len(events) != 7 {
		t.Fatalf("event count = %d, want 7", len(events))
	}
}
