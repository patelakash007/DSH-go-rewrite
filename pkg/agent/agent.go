//

//

package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/llm"
	"github.com/JopenChen/dsh-go/pkg/session"
	"github.com/JopenChen/dsh-go/pkg/sysprompt"
	"github.com/JopenChen/dsh-go/pkg/tools"
)

var ErrTurnQueueFull = errors.New("agent: turn queue full, reject pending turn")

type CancelCause string

const (
	CancelUser CancelCause = "user"

	CancelParent CancelCause = "parent"

	CancelHook CancelCause = "hook"

	CancelDisposed CancelCause = "disposed"

	CancelLegacy CancelCause = "legacy"
)

type Agent struct {
	ID brand.SessionID

	log *session.SessionLog

	sys *sysprompt.Assembler

	pipeline *tools.Pipeline

	adapter llm.LLMAdapter

	options AgentOptions

	inbox *Inbox

	turnCh chan *turnReq

	stopCh chan struct{}

	wg sync.WaitGroup

	started bool

	mu sync.Mutex

	status AgentStatus

	idleCh chan struct{}

	pendingToolRounds int

	toolProvider func(name string) *tools.Tool

	runCtx context.Context
}

type turnReq struct {
	// kind "run" / "followup"。
	kind string

	input string

	done chan error
}

func NewAgent(id brand.SessionID, log *session.SessionLog, sys *sysprompt.Assembler, pipeline *tools.Pipeline, adapter llm.LLMAdapter, options ...AgentOptions) *Agent {
	opts := AgentOptions{}
	if len(options) > 0 {
		opts = options[0]
	}
	a := &Agent{
		ID:       id,
		log:      log,
		sys:      sys,
		pipeline: pipeline,
		adapter:  adapter,
		options:  opts,
		turnCh:   make(chan *turnReq, 2),
		stopCh:   make(chan struct{}),
		status:   StatusIdle,
		idleCh:   make(chan struct{}),
	}

	close(a.idleCh)

	a.inbox = NewInbox(log, InboxNotifications{})
	return a
}

func (a *Agent) Inbox() *Inbox {
	return a.inbox
}

func (a *Agent) Options() AgentOptions {
	return a.options
}

func (a *Agent) Status() AgentStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.status
}

func (a *Agent) setStatus(status AgentStatus) {
	a.mu.Lock()
	if a.status == status {
		a.mu.Unlock()
		return
	}
	a.status = status
	if status == StatusRunning {

		a.idleCh = make(chan struct{})
	} else if status == StatusIdle {

		close(a.idleCh)
	}
	a.mu.Unlock()

	_, _ = a.log.Append(session.AgentStatusData{Status: string(status)})
}

func (a *Agent) WhenIdle(ctx context.Context) error {
	a.mu.Lock()
	idleCh := a.idleCh
	a.mu.Unlock()

	select {
	case <-idleCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (a *Agent) SetToolProvider(fn func(name string) *tools.Tool) {
	a.toolProvider = fn
}

func (a *Agent) SetRunContext(parent context.Context, timeout time.Duration) {
	if parent == nil {
		parent = context.Background()
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		parent, cancel = context.WithTimeout(parent, timeout)
		_ = cancel
	}
	a.runCtx = parent
}

func (a *Agent) runContext() context.Context {
	if a.runCtx == nil {
		return context.Background()
	}
	return a.runCtx
}

func (a *Agent) Log() *session.SessionLog {
	return a.log
}

func (a *Agent) ensureStarted() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.started {
		a.started = true
		a.wg.Add(1)
		go a.loop()
	}
}

func (a *Agent) loop() {
	defer a.wg.Done()
	for {
		select {
		case <-a.stopCh:
			return
		case req := <-a.turnCh:
			a.runTurn(req)
		}
	}
}

func (a *Agent) Run(input string) error {
	a.ensureStarted()
	req := &turnReq{kind: "run", input: input, done: make(chan error, 1)}
	select {
	case a.turnCh <- req:
		return nil
	default:
		return ErrTurnQueueFull
	}
}

func (a *Agent) Followup(input string) error {
	a.ensureStarted()
	req := &turnReq{kind: "followup", input: input, done: make(chan error, 1)}
	select {
	case a.turnCh <- req:
		return nil
	default:
		return ErrTurnQueueFull
	}
}

func (a *Agent) Dispose() {
	a.mu.Lock()
	if !a.started {
		a.mu.Unlock()
		return
	}
	close(a.stopCh)
	a.started = false
	a.mu.Unlock()
	a.wg.Wait()
}

func (a *Agent) Cancel(cause CancelCause) {
	_ = RecordCancel(a.log, cause)
}

func (a *Agent) runTurn(req *turnReq) {
	var turnErr error
	defer func() {

		a.setStatus(StatusIdle)
		if req.done != nil {
			req.done <- turnErr
		}
	}()

	a.setStatus(StatusRunning)

	turnIdx := a.log.NextTurn()

	// turn/start
	if _, err := a.log.Append(session.TurnStartData{Turn: turnIdx}); err != nil {
		turnErr = err
		return
	}

	if _, err := a.log.Append(session.UserMessageData{Content: req.input, Source: req.kind}); err != nil {
		_ = a.failTurn(turnIdx, turnErr, err)
		turnErr = err
		return
	}

	stepSeq := uint64(1)
	for {
		// step/start
		if _, err := a.log.Append(session.StepStartData{Turn: turnIdx, Step: stepSeq}); err != nil {
			_ = a.failTurn(turnIdx, turnErr, err)
			turnErr = err
			return
		}

		err := a.runStep(req, stepSeq)
		// step/end
		_, _ = a.log.Append(session.StepEndData{Turn: turnIdx, Step: stepSeq})

		if err != nil {

			_ = a.failTurn(turnIdx, turnErr, err)
			turnErr = err
			return
		}

		cont := a.lastStepHadToolCall()
		if !cont {
			break
		}
		stepSeq++
	}

	// turn-stopping → turn/end (finished)
	_, _ = a.log.Append(session.TurnStoppingData{Reason: "finished"})
	_, _ = a.log.Append(session.TurnEndData{Turn: turnIdx, Reason: session.ReasonFinished})
}

func (a *Agent) lastStepHadToolCall() bool {
	return a.pendingToolRounds > 0
}

func (a *Agent) failTurn(turnIdx uint64, prev, err error) error {
	_, _ = a.log.Append(session.AgentErrorData{Message: err.Error(), Pkg: "pkg/agent"})
	_, _ = a.log.Append(session.TurnEndData{Turn: turnIdx, Reason: session.ReasonInterrupted})
	return err
}

func (a *Agent) runStep(req *turnReq, stepSeq uint64) error {
	// agent/request
	_, _ = a.log.Append(session.AgentRequestData{Provider: a.adapterName(), Model: a.modelName()})

	system := ""
	if a.sys != nil {
		system = a.sys.Assemble()
	}
	history := session.DeriveMessages(a.log.Events())
	messages := make([]llm.Message, 0, len(history))
	for _, m := range history {
		if m.Role == "user" {
			messages = append(messages, llm.NewUserMessage(m.Content))
		} else {
			messages = append(messages, llm.NewAssistantText(m.Content))
		}
	}

	var toolCalls []*llm.ToolCall
	runCtx := a.runContext()
	_, err := a.adapter.Chat(runCtx, llm.ChatRequest{
		Model:    a.modelName(),
		System:   system,
		Messages: messages,
		Tools:    a.toolSchemas(),
	}, func(chunk llm.StreamChunk) {
		switch chunk.Kind {
		case llm.ChunkText:
			_, _ = a.log.Append(session.AssistantChunkData{Text: chunk.Text})
		case llm.ChunkReasoning:
			_, _ = a.log.Append(session.AssistantReasoningData{Text: chunk.Reasoning})
		case llm.ChunkToolCall:
			if chunk.ToolCall != nil {
				toolCalls = append(toolCalls, chunk.ToolCall)
			}
		}
	})
	if err != nil {
		return fmt.Errorf("llm step failed: %w", err)
	}

	if len(toolCalls) > 0 {
		a.pendingToolRounds = len(toolCalls)
		for _, tc := range toolCalls {
			if err := a.executeTool(tc); err != nil {
				return err
			}
		}
		a.pendingToolRounds = len(toolCalls)
	} else {
		a.pendingToolRounds = 0
	}
	return nil
}

func (a *Agent) executeTool(tc *llm.ToolCall) error {
	callID := brand.NewToolCallID(tc.ID)
	// tool/call
	if _, err := a.log.Append(session.ToolCallData{CallID: callID, Tool: tc.Name, Input: toRaw(tc.Input)}); err != nil {
		return err
	}

	if a.pipeline == nil {

		_, _ = a.log.Append(session.ToolResultData{CallID: callID, IsError: false, Output: "(no pipeline)"})
		return nil
	}

	req := &tools.ToolCallRequest{CallID: callID, Tool: tc.Name, Input: tc.Input}
	res := a.pipeline.Run(a.runContext(), req, a.findTool(tc.Name))
	_, err := a.log.Append(session.ToolResultData{
		CallID: callID, IsError: res.IsError, Output: stringify(res.Value),
	})
	return err
}

func (a *Agent) adapterName() string {
	if a.adapter == nil {
		return "unknown"
	}
	return a.adapter.Name()
}

func (a *Agent) modelName() string {
	return "deepseek-chat"
}

func (a *Agent) toolSchemas() []llm.ToolSchema {
	return nil
}

func (a *Agent) findTool(name string) *tools.Tool {
	if a.toolProvider != nil {
		return a.toolProvider(name)
	}
	return nil
}

func jsonMarshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

func toRaw(input map[string]any) []byte {
	if input == nil {
		return nil
	}
	b, _ := jsonMarshal(input)
	return b
}

func stringify(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	b, _ := jsonMarshal(v)
	return string(b)
}
