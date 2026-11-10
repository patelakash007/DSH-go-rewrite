package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/JopenChen/dsh-go/pkg/agent"
	"github.com/JopenChen/dsh-go/pkg/tools"
	"github.com/JopenChen/dsh-go/pkg/uuid"
)

const questionWaitTimeout = 10 * time.Minute

type pendingQuestion struct {
	id        string
	sessionID string
	callID    string
	questions []map[string]any
	createdAt time.Time
	result    chan map[string]any
	once      sync.Once
}

func (s *Server) askUserQuestionTool(sessionID string) *tools.Tool {
	allow := false
	return &tools.Tool{
		Name:        "ask_user_question",
		Description: "Ask the user one or more structured questions and wait for their answers. Use it when a decision or missing detail blocks progress.",
		Schema: &tools.JsonSchemaNode{
			Type: tools.TypeObject,
			Properties: map[string]*tools.JsonSchemaNode{
				"questions": {
					Type:        tools.TypeArray,
					Description: "Questions to show as one card",
					Items: &tools.JsonSchemaNode{
						Type: tools.TypeObject,
						Properties: map[string]*tools.JsonSchemaNode{
							"id":       {Type: tools.TypeString, Description: "Stable id used in the answer"},
							"question": {Type: tools.TypeString, Description: "Question text"},
							"detail":   {Type: tools.TypeString, Description: "Optional markdown detail"},
							"header":   {Type: tools.TypeString, Description: "Optional short header"},
							"options": {
								Type: tools.TypeArray,
								Items: &tools.JsonSchemaNode{
									Type: tools.TypeObject,
									Properties: map[string]*tools.JsonSchemaNode{
										"label":       {Type: tools.TypeString},
										"description": {Type: tools.TypeString},
									},
									Required: []string{"label"}, AdditionalProperties: &allow,
								},
							},
							"multiSelect": {Type: tools.TypeBoolean, Description: "Allow several options"},
						},
						Required: []string{"id", "question"}, AdditionalProperties: &allow,
					},
				},
			},
			Required: []string{"questions"}, AdditionalProperties: &allow,
		},
		Execute: func(ctx context.Context, input map[string]any) (any, error) {
			return s.askUserQuestion(ctx, sessionID, input)
		},
	}
}

func (s *Server) askUserQuestion(ctx context.Context, sessionID string, input map[string]any) (any, error) {
	rawQuestions, _ := input["questions"].([]any)
	questions := make([]map[string]any, 0, len(rawQuestions))
	for _, raw := range rawQuestions {
		question, _ := raw.(map[string]any)
		if question == nil {
			continue
		}
		id, _ := question["id"].(string)
		text, _ := question["question"].(string)
		if strings.TrimSpace(id) == "" || strings.TrimSpace(text) == "" {
			return nil, fmt.Errorf("ask_user_question requires an id and question text for every question")
		}
		questions = append(questions, question)
	}
	if len(questions) == 0 {
		return nil, fmt.Errorf("ask_user_question requires at least one question")
	}
	eventID := uuid.MustV4()
	callID := uuid.MustV4()
	pending := &pendingQuestion{
		id: eventID, sessionID: sessionID, callID: callID,
		questions: questions, createdAt: time.Now(), result: make(chan map[string]any, 1),
	}
	frame := map[string]any{
		"type": "waterfall", "event": "user-questions/request", "eventId": eventID, "agentId": sessionID,
		"request": map[string]any{
			"questions": questions,
			"wait":      map[string]any{"callId": callID, "timed": false},
		},
	}
	s.hostMu.Lock()
	s.pendingQuestions[eventID] = pending
	s.hostMu.Unlock()
	s.emitHost(frame)

	waitCtx, cancel := context.WithTimeout(ctx, questionWaitTimeout)
	defer cancel()
	var value map[string]any
	select {
	case value = <-pending.result:
	case <-waitCtx.Done():
	}
	s.hostMu.Lock()
	if s.pendingQuestions[eventID] == pending {
		delete(s.pendingQuestions, eventID)
	}
	s.hostMu.Unlock()
	s.emitHost(map[string]any{"type": "cancel", "eventId": eventID})
	if value == nil {
		s.hostMu.Lock()
		s.continuedQuestions[sessionID+"\x00"+callID] = pending
		s.hostMu.Unlock()
		return map[string]any{"status": "continued", "callId": callID}, nil
	}
	return map[string]any{"status": "answered", "answers": value["answers"]}, nil
}

func (s *Server) resolveQuestion(eventID string, outcome map[string]any) bool {
	s.hostMu.Lock()
	pending := s.pendingQuestions[eventID]
	if pending == nil {
		s.hostMu.Unlock()
		return false
	}
	delete(s.pendingQuestions, eventID)
	s.hostMu.Unlock()
	kind, _ := outcome["kind"].(string)
	value := map[string]any{}
	if kind == "result" {
		value, _ = outcome["value"].(map[string]any)
	}
	if value == nil {
		value = map[string]any{}
	}
	pending.once.Do(func() { pending.result <- value })
	return true
}

func (s *Server) dispatchUserQuestionRPC(endpoint string, args map[string]any) (any, *RPCError) {
	switch endpoint {
	case "userQuestions/answer":
		sessionID, _ := args["agentId"].(string)
		callID, _ := args["callId"].(string)
		answer, _ := args["answer"].(map[string]any)
		answers, _ := answer["answers"].([]any)
		s.hostMu.Lock()
		pending := s.continuedQuestions[sessionID+"\x00"+callID]
		if pending != nil {
			delete(s.continuedQuestions, sessionID+"\x00"+callID)
		}
		s.hostMu.Unlock()
		if pending == nil {
			return false, nil
		}
		rec := s.sessionByID(sessionID)
		if rec == nil {
			return false, nil
		}
		payload, _ := json.Marshal(map[string]any{"callId": callID, "answers": answers})
		runtimeAgent := s.ensureAgent(rec)
		rec.mu.Lock()
		running := rec.Cancel != nil
		rec.Cancel = func() { runtimeAgent.Cancel(agent.CancelUser) }
		rec.mu.Unlock()
		var err error
		if running || runtimeAgent.Status() == agent.StatusRunning {
			err = runtimeAgent.FollowupRequest("Answers to your earlier question: "+string(payload), uuid.MustV4())
		} else {
			err = runtimeAgent.RunRequest("Answers to your earlier question: "+string(payload), uuid.MustV4())
		}
		if err != nil {
			return false, nil
		}
		return true, nil
	default:
		return nil, &RPCError{Code: "gateway/method-not-found", Message: "unknown user question method"}
	}
}

func (s *Server) openQuestionWait(sessionID, callID string, send func(map[string]any) error) {
	deadline := time.Now().Add(questionWaitTimeout)
	for {
		s.hostMu.Lock()
		pending := s.continuedQuestions[sessionID+"\x00"+callID]
		if pending == nil {
			for _, candidate := range s.pendingQuestions {
				if candidate.sessionID == sessionID && candidate.callID == callID {
					pending = candidate
					break
				}
			}
		}
		s.hostMu.Unlock()
		if pending == nil {
			return
		}
		remaining := time.Until(deadline).Milliseconds()
		if remaining < 0 {
			remaining = 0
		}
		if err := send(map[string]any{"remainingMs": remaining}); err != nil {
			return
		}
		if remaining == 0 {
			return
		}
		time.Sleep(time.Second)
	}
}
