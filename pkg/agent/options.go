//

package agent

import (
	"time"
)

//

type AgentOptions struct {
	Provider string `json:"provider,omitempty"`

	Model string `json:"model,omitempty"`

	ReasoningEffort string `json:"reasoningEffort,omitempty"`

	MaxTokens int `json:"maxTokens,omitempty"`

	DefaultTimeout time.Duration `json:"defaultTimeout,omitempty"`
}

//

//

type AgentStatus string

const (
	StatusIdle AgentStatus = "idle"

	StatusRunning AgentStatus = "running"
)

type CancelOptions struct {
	KeepInbox bool `json:"keepInbox,omitempty"`
}

type PreStepDecision struct {
	Kind string `json:"kind"`

	Messages []UserMessage `json:"messages,omitempty"`

	StartsRequestSeries bool `json:"startsRequestSeries,omitempty"`
}

func RejectPreStep() PreStepDecision {
	return PreStepDecision{Kind: "reject"}
}

func EnterPreStep(messages ...UserMessage) PreStepDecision {
	return PreStepDecision{Kind: "enter", Messages: messages}
}

//

type SessionStartSource string

const (
	StartSourceStartup SessionStartSource = "startup"

	StartSourceResume SessionStartSource = "resume"

	StartSourceClear SessionStartSource = "clear"

	StartSourceCompact SessionStartSource = "compact"
)

type UserMessage struct {
	ID string `json:"id"`

	Content string `json:"content"`

	Source string `json:"source,omitempty"`

	CreatedAt time.Time `json:"createdAt,omitempty"`
}
