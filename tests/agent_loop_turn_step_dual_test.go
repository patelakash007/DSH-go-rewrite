package tests

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/JopenChen/dsh-go/pkg/agent"
	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/llm"
	"github.com/JopenChen/dsh-go/pkg/session"
	"github.com/JopenChen/dsh-go/pkg/sysprompt"
	"github.com/JopenChen/dsh-go/pkg/tools"
)

type scriptedAdapter struct {
	mu       sync.Mutex
	scripts  [][]llm.StreamChunk
	order    []string
	callIdx  int
	blockOn  chan struct{}
	released bool
	failOn   int
}

func newScriptedAdapter() *scriptedAdapter {
	return &scriptedAdapter{}
}

func (s *scriptedAdapter) blockFirst() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.blockOn == nil {
		s.blockOn = make(chan struct{})
	}
}

func (s *scriptedAdapter) script(chunks ...llm.StreamChunk) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scripts = append(s.scripts, chunks)
}

func (s *scriptedAdapter) Name() string { return "scripted" }

func (s *scriptedAdapter) release() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.released {
		s.released = true
		close(s.blockOn)
	}
}

func (s *scriptedAdapter) Chat(ctx context.Context, req llm.ChatRequest, cb func(llm.StreamChunk)) (llm.Usage, error) {
	s.mu.Lock()

	lastUser := ""
	for _, m := range req.Messages {
		if m.Role == llm.RoleUser {
			for _, b := range m.Content {
				if b.Kind == llm.BlockText {
					lastUser = b.Text
				}
			}
		}
	}
	s.order = append(s.order, lastUser)
	s.callIdx++
	idx := s.callIdx
	block := s.blockOn
	failOn := s.failOn
	chunks := []llm.StreamChunk{}
	if idx <= len(s.scripts) {
		chunks = s.scripts[idx-1]
	}
	s.mu.Unlock()

	if block != nil && idx == 1 {
		select {
		case <-block:
		case <-ctx.Done():
			return llm.Usage{}, ctx.Err()
		}
	}

	if failOn != 0 && idx == failOn {
		return llm.Usage{}, errors.New("injected llm failure")
	}

	for _, c := range chunks {
		if cb != nil {
			cb(c)
		}
	}
	if cb != nil {
		cb(llm.StreamChunk{Kind: llm.ChunkDone})
	}
	return llm.Usage{PromptTokens: 1, CompletionTokens: 1}, nil
}

func (s *scriptedAdapter) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.callIdx
}

func buildAgent(t *testing.T, adapter *scriptedAdapter) *agent.Agent {
	t.Helper()
	sl := session.NewSessionLog(brand.NewSessionID("agent_1"))
	sys := sysprompt.New()

	echo := &tools.Tool{
		Name:        "echo",
		Description: "echo input",
		Execute: func(ctx context.Context, input map[string]any) (any, error) {
			return map[string]any{"echo": input["msg"]}, nil
		},
	}
	pipeline := tools.NewPipeline().WithTool(echo)

	a := agent.NewAgent(brand.NewSessionID("agent_1"), sl, sys, pipeline, adapter)
	a.SetToolProvider(func(name string) *tools.Tool {
		if name == "echo" {
			return echo
		}
		return nil
	})
	return a
}

func waitFor(cond func() bool) bool {
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

func countTurnEnds(sl *session.SessionLog) int {
	n := 0
	for _, ev := range sl.Events() {
		if ev.Type == session.EventTurnEnd {
			n++
		}
	}
	return n
}

func lastEventOfType(sl *session.SessionLog, typ session.EventType) (session.SessionEvent, bool) {
	events := sl.Events()
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Type == typ {
			return events[i], true
		}
	}
	return session.SessionEvent{}, false
}

func TestAgentTurnStepTextOnly(t *testing.T) {
	adapter := newScriptedAdapter()
	adapter.script(llm.StreamChunk{Kind: llm.ChunkText, Text: "hello"})
	a := buildAgent(t, adapter)

	if err := a.Run("hello"); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if !waitFor(func() bool { return countTurnEnds(a.Log()) == 1 }) {
		t.Fatal("turn did not complete within timeout")
	}

	ev, ok := lastEventOfType(a.Log(), session.EventTurnEnd)
	if !ok {
		t.Fatal("missing turn/end")
	}
	if td, ok := ev.Data.(session.TurnEndData); !ok || td.Reason != session.ReasonFinished {
		t.Fatalf("turn/end should be finished: %+v", ev.Data)
	}
}

func TestAgentTurnWithTool(t *testing.T) {
	adapter := newScriptedAdapter()

	adapter.script(llm.StreamChunk{Kind: llm.ChunkToolCall, ToolCall: &llm.ToolCall{ID: "call_1", Name: "echo", Input: map[string]any{"msg": "hi"}}})

	adapter.script(llm.StreamChunk{Kind: llm.ChunkText, Text: "done"})
	a := buildAgent(t, adapter)

	if err := a.Run("use echo"); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if !waitFor(func() bool { return countTurnEnds(a.Log()) == 1 }) {
		t.Fatal("turn did not complete within timeout")
	}

	hasCall, hasResult := false, false
	for _, ev := range a.Log().Events() {
		switch ev.Type {
		case session.EventToolCall:
			hasCall = true
		case session.EventToolResult:
			hasResult = true
		}
	}
	if !hasCall || !hasResult {
		t.Fatalf("tool/call and tool/result should exist: call=%v result=%v", hasCall, hasResult)
	}

	if adapter.callCount() != 2 {
		t.Fatalf("Chat call count = %d, want 2", adapter.callCount())
	}
}

func TestAgentFollowupSerialQueue(t *testing.T) {
	adapter := newScriptedAdapter()
	adapter.blockFirst()

	adapter.script(llm.StreamChunk{Kind: llm.ChunkText, Text: "r1"})
	adapter.script(llm.StreamChunk{Kind: llm.ChunkText, Text: "r2"})
	adapter.script(llm.StreamChunk{Kind: llm.ChunkText, Text: "r3"})
	a := buildAgent(t, adapter)

	if err := a.Run("first"); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if !waitFor(func() bool { return adapter.callCount() >= 1 }) {
		t.Fatal("first Chat did not start")
	}

	if err := a.Followup("f1"); err != nil {
		t.Fatalf("Followup f1 should queue successfully: %v", err)
	}
	if err := a.Followup("f2"); err != nil {
		t.Fatalf("Followup f2 should queue successfully: %v", err)
	}

	if err := a.Followup("f3"); err != nil {
		if err != agent.ErrTurnQueueFull {
			t.Fatalf("item 3 should reject as ErrTurnQueueFull, got %v", err)
		}
	} else {
		t.Fatal("item 3 Followup should be rejected")
	}

	adapter.release()
	if !waitFor(func() bool { return countTurnEnds(a.Log()) == 3 }) {
		t.Fatal("3 turns did not complete within timeout")
	}

	adapter.mu.Lock()
	order := append([]string(nil), adapter.order...)
	adapter.mu.Unlock()
	if len(order) != 3 || order[0] != "first" || order[1] != "f1" || order[2] != "f2" {
		t.Fatalf("serial ordering unexpected: %v", order)
	}
}

func TestAgentStepErrorInterruptsTurn(t *testing.T) {
	adapter := newScriptedAdapter()
	adapter.failOn = 1
	a := buildAgent(t, adapter)

	if err := a.Run("trigger error"); err != nil {
		t.Fatalf("Run enqueue failed: %v", err)
	}
	if !waitFor(func() bool { return countTurnEnds(a.Log()) == 1 }) {
		t.Fatal("turn did not complete within timeout")
	}

	if _, ok := lastEventOfType(a.Log(), session.EventAgentError); !ok {
		t.Fatal("missing agent/error event")
	}

	ev, _ := lastEventOfType(a.Log(), session.EventTurnEnd)
	if td, ok := ev.Data.(session.TurnEndData); !ok || td.Reason != session.ReasonInterrupted {
		t.Fatalf("turn/end should be interrupted: %+v", ev.Data)
	}
}

func TestAgentDispose(t *testing.T) {
	adapter := newScriptedAdapter()
	adapter.script(llm.StreamChunk{Kind: llm.ChunkText, Text: "x"})
	a := buildAgent(t, adapter)
	_ = a.Run("x")
	if !waitFor(func() bool { return countTurnEnds(a.Log()) == 1 }) {
		t.Fatal("turn not completed")
	}
	a.Dispose()
}
