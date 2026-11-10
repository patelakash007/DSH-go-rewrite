//

//

package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
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
	toolRegistry map[string]*tools.Tool
	toolOrder    []string

	runCtx        context.Context
	runTimeout    time.Duration
	activeCancel  context.CancelFunc
	activeRequest string
}

type turnReq struct {
	// kind "run" / "followup"。
	kind string

	input     string
	requestID string
	context   string

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
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.options
}

func (a *Agent) SetModel(provider, model, reasoningEffort string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if provider != "" {
		a.options.Provider = provider
	}
	if model != "" {
		a.options.Model = model
	}
	if reasoningEffort != "" {
		a.options.ReasoningEffort = reasoningEffort
	}
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
	a.mu.Lock()
	defer a.mu.Unlock()
	a.toolProvider = fn
}

// SetTools installs the model-visible tool catalogue. The provider callback
// remains available for dynamic implementations, but only tools in this
// catalogue are advertised to the model.
func (a *Agent) SetTools(definitions ...*tools.Tool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.toolRegistry = make(map[string]*tools.Tool, len(definitions))
	a.toolOrder = a.toolOrder[:0]
	for _, tool := range definitions {
		if tool == nil || tool.Name == "" {
			continue
		}
		if _, exists := a.toolRegistry[tool.Name]; !exists {
			a.toolOrder = append(a.toolOrder, tool.Name)
		}
		a.toolRegistry[tool.Name] = tool
	}
	sort.Strings(a.toolOrder)
}

func (a *Agent) SetRunContext(parent context.Context, timeout time.Duration) {
	if parent == nil {
		parent = context.Background()
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.runCtx = parent
	a.runTimeout = timeout
}

func (a *Agent) beginRunContext(requestID string) (context.Context, context.CancelFunc) {
	a.mu.Lock()
	parent := a.runCtx
	timeout := a.runTimeout
	if timeout <= 0 {
		timeout = a.options.DefaultTimeout
	}
	if parent == nil {
		parent = context.Background()
	}
	var ctx context.Context
	var cancel context.CancelFunc
	if timeout > 0 {
		ctx, cancel = context.WithTimeout(parent, timeout)
	} else {
		ctx, cancel = context.WithCancel(parent)
	}
	a.activeCancel = cancel
	a.activeRequest = requestID
	a.mu.Unlock()
	return ctx, cancel
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
	return a.RunRequest(input, "")
}

func (a *Agent) RunRequest(input, requestID string) error {
	return a.RunRequestWithContext(input, requestID, "")
}

// RunRequestWithContext queues a turn with one-turn system context (for
// example a referenced-conversation snapshot).
func (a *Agent) RunRequestWithContext(input, requestID, context string) error {
	a.ensureStarted()
	req := &turnReq{kind: "run", input: input, requestID: requestID, context: context, done: make(chan error, 1)}
	select {
	case a.turnCh <- req:
		return nil
	default:
		return ErrTurnQueueFull
	}
}

// RunRequestWait queues a turn and waits for that exact turn to finish. It is
// used by synchronous callers such as the native subagent tool.
func (a *Agent) RunRequestWait(ctx context.Context, input, requestID string) error {
	a.ensureStarted()
	req := &turnReq{kind: "run", input: input, requestID: requestID, done: make(chan error, 1)}
	select {
	case a.turnCh <- req:
	default:
		return ErrTurnQueueFull
	}
	select {
	case err := <-req.done:
		return err
	case <-ctx.Done():
		a.Cancel(CancelUser)
		return ctx.Err()
	}
}

func (a *Agent) Followup(input string) error {
	return a.FollowupRequest(input, "")
}

func (a *Agent) FollowupRequest(input, requestID string) error {
	return a.FollowupRequestWithContext(input, requestID, "")
}

// FollowupRequestWithContext queues a follow-up turn with one-turn system
// context.
func (a *Agent) FollowupRequestWithContext(input, requestID, context string) error {
	a.ensureStarted()
	req := &turnReq{kind: "followup", input: input, requestID: requestID, context: context, done: make(chan error, 1)}
	select {
	case a.turnCh <- req:
		return nil
	default:
		return ErrTurnQueueFull
	}
}

func (a *Agent) Dispose() {
	a.Cancel(CancelDisposed)
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
	a.mu.Lock()
	cancel := a.activeCancel
	a.mu.Unlock()
	if _, open := a.log.OpenTurn(); open {
		_ = RecordCancel(a.log, cause)
	}
	if cancel != nil {
		cancel()
	}
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
	ctx, cancel := a.beginRunContext(req.requestID)
	defer func() {
		cancel()
		a.mu.Lock()
		if a.activeRequest == req.requestID {
			a.activeCancel = nil
			a.activeRequest = ""
		}
		a.mu.Unlock()
	}()

	turnIdx := a.log.NextTurn()

	// turn/start
	if _, err := a.log.Append(session.TurnStartData{Turn: turnIdx}); err != nil {
		turnErr = err
		return
	}

	if _, err := a.log.Append(session.UserMessageData{Content: req.input, Source: req.kind, RequestID: req.requestID}); err != nil {
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

		hadTools, err := a.runStep(ctx, req, turnIdx, stepSeq)
		// step/end
		_, _ = a.log.Append(session.StepEndData{Turn: turnIdx, Step: stepSeq})

		if err != nil {
			if ctx.Err() != nil {
				_, _ = a.log.Append(session.TurnEndData{Turn: turnIdx, Reason: session.ReasonAborted})
			} else {
				_ = a.failTurn(turnIdx, turnErr, err)
			}
			turnErr = err
			return
		}

		if !hadTools {
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

func (a *Agent) runStep(ctx context.Context, req *turnReq, turnIdx, stepSeq uint64) (bool, error) {
	// agent/request
	_, _ = a.log.Append(session.AgentRequestData{Provider: a.adapterName(), Model: a.modelName()})

	system := ""
	if a.sys != nil {
		system = a.sys.Assemble()
	}
	if req.context != "" {
		system += "\n\n" + req.context
	}
	messages := deriveLLMMessages(a.log.Events())

	var toolCalls []*llm.ToolCall
	var text strings.Builder
	var reasoning strings.Builder
	usage, err := a.adapter.Chat(ctx, llm.ChatRequest{
		Provider:  a.providerName(),
		Model:     a.modelName(),
		System:    system,
		Messages:  messages,
		Tools:     a.toolSchemas(),
		MaxTokens: a.options.MaxTokens,
	}, func(chunk llm.StreamChunk) {
		if ctx.Err() != nil {
			return
		}
		switch chunk.Kind {
		case llm.ChunkText:
			text.WriteString(chunk.Text)
			_, _ = a.log.Append(session.AssistantChunkData{Text: chunk.Text})
		case llm.ChunkReasoning:
			reasoning.WriteString(chunk.Reasoning)
			_, _ = a.log.Append(session.AssistantReasoningData{Text: chunk.Reasoning})
		case llm.ChunkToolCall:
			if chunk.ToolCall != nil {
				toolCalls = append(toolCalls, chunk.ToolCall)
			}
		}
	})
	if err != nil {
		return false, fmt.Errorf("llm step failed: %w", err)
	}

	committedCalls := make([]session.AssistantToolCall, 0, len(toolCalls))
	callIDs := make([]string, 0, len(toolCalls))
	for _, call := range toolCalls {
		if call == nil {
			continue
		}
		committedCalls = append(committedCalls, session.AssistantToolCall{ID: call.ID, Name: call.Name, Input: call.Input})
		callIDs = append(callIDs, call.ID)
	}
	if _, err := a.log.Append(session.AssistantMessageData{
		Turn: turnIdx, Step: stepSeq, Content: text.String(), Reasoning: reasoning.String(),
		ToolCallIDs: callIDs, ToolCalls: committedCalls,
		PromptTokens: usage.PromptTokens, OutputTokens: usage.CompletionTokens,
	}); err != nil {
		return false, err
	}

	if len(toolCalls) > 0 {
		a.pendingToolRounds = len(toolCalls)
		for _, tc := range toolCalls {
			if err := a.executeTool(ctx, turnIdx, stepSeq, tc); err != nil {
				return false, err
			}
		}
		a.pendingToolRounds = len(toolCalls)
	} else {
		a.pendingToolRounds = 0
	}
	return len(toolCalls) > 0, nil
}

func (a *Agent) executeTool(ctx context.Context, turnIdx, stepSeq uint64, tc *llm.ToolCall) error {
	if tc == nil {
		return nil
	}
	callID := brand.NewToolCallID(tc.ID)
	// tool/call
	if _, err := a.log.Append(session.ToolCallData{Turn: turnIdx, Step: stepSeq, CallID: callID, Tool: tc.Name, Input: toRaw(tc.Input)}); err != nil {
		return err
	}

	tool := a.findTool(tc.Name)
	if a.pipeline == nil {
		if tool == nil || tool.Execute == nil {
			_, err := a.log.Append(session.ToolResultData{Turn: turnIdx, Step: stepSeq, CallID: callID, IsError: true, Output: "tool implementation missing"})
			return err
		}
		value, runErr := tool.Execute(ctx, tc.Input)
		output := stringify(value)
		if runErr != nil {
			output = runErr.Error()
		}
		_, err := a.log.Append(session.ToolResultData{Turn: turnIdx, Step: stepSeq, CallID: callID, IsError: runErr != nil, Output: output})
		return err
	}

	req := &tools.ToolCallRequest{CallID: callID, Tool: tc.Name, Input: tc.Input}
	res := a.pipeline.Run(ctx, req, tool)
	output := stringify(res.Value)
	if res.IsError && res.Error != "" {
		output = res.Error
	}
	_, err := a.log.Append(session.ToolResultData{
		Turn: turnIdx, Step: stepSeq, CallID: callID, IsError: res.IsError, Output: output,
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
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.options.Model != "" {
		return a.options.Model
	}
	return "deepseek-chat"
}

func (a *Agent) providerName() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.options.Provider != "" {
		return a.options.Provider
	}
	return "deepseek"
}

func (a *Agent) toolSchemas() []llm.ToolSchema {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]llm.ToolSchema, 0, len(a.toolOrder))
	for _, name := range a.toolOrder {
		tool := a.toolRegistry[name]
		if tool == nil {
			continue
		}
		parameters := map[string]any{"type": "object", "properties": map[string]any{}}
		if tool.Schema != nil {
			raw, err := json.Marshal(tool.Schema)
			if err == nil {
				_ = json.Unmarshal(raw, &parameters)
			}
		}
		out = append(out, llm.ToolSchema{Name: tool.Name, Description: tool.Description, Parameters: parameters})
	}
	return out
}

func (a *Agent) findTool(name string) *tools.Tool {
	a.mu.Lock()
	provider := a.toolProvider
	registered := a.toolRegistry[name]
	a.mu.Unlock()
	if provider != nil {
		if tool := provider(name); tool != nil {
			return tool
		}
	}
	return registered
}

func deriveLLMMessages(events []session.SessionEvent) []llm.Message {
	messages := make([]llm.Message, 0)
	for _, event := range session.EffectiveEvents(events) {
		switch data := event.Data.(type) {
		case session.UserMessageData:
			messages = append(messages, llm.NewUserMessage(data.Content))
		case session.AssistantMessageData:
			calls := make([]*llm.ToolCall, 0, len(data.ToolCalls))
			for _, call := range data.ToolCalls {
				calls = append(calls, &llm.ToolCall{ID: call.ID, Name: call.Name, Input: call.Input})
			}
			messages = append(messages, llm.NewAssistantMessage(data.Content, calls...))
		case session.ToolResultData:
			messages = append(messages, llm.NewToolResultMessage(data.CallID.Raw(), data.Output, data.IsError))
		}
	}
	return messages
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
