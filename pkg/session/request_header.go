//

//

package session

import (
	"encoding/json"
	"fmt"
	"strings"
)

type Reason string

const (
	ReasonInitial Reason = "initial"
	ReasonResume  Reason = "resume"
	ReasonChange  Reason = "change"
	ReasonSeries  Reason = "series"
)

type EpochHeader struct {
	ConfigEpoch uint64 `json:"configEpoch"`

	SystemHash string `json:"systemHash"`

	ToolCount int `json:"toolCount"`
}

type RequestContext struct {
	Provider string `json:"provider"`

	Model string `json:"model"`

	ContextWindow int `json:"contextWindow,omitempty"`

	Reason Reason `json:"reason,omitempty"`
}

type RequestHeaderSnapshot struct {
	Header EpochHeader `json:"header"`

	Context RequestContext `json:"context"`

	SystemPrompt string `json:"systemPrompt,omitempty"`

	ToolSchemas json.RawMessage `json:"toolSchemas,omitempty"`
}

func FoldRequestContext(events []SessionEvent) (RequestContext, bool) {
	var ctx RequestContext
	ok := false
	for _, ev := range events {
		if ev.Type != EventRequestContext {
			continue
		}
		if d, isData := ev.Data.(RequestContextData); isData {
			ctx = RequestContext{
				Provider:      d.Provider,
				Model:         d.Model,
				ContextWindow: d.Window,
				Reason:        Reason(d.Reason),
			}
			ok = true
		}
	}
	return ctx, ok
}

func FoldEpochHeader(events []SessionEvent) (EpochHeader, bool) {
	var h EpochHeader
	ok := false
	for _, ev := range events {
		if ev.Type != EventRequestHeader {
			continue
		}
		if d, isData := ev.Data.(RequestHeaderData); isData {
			h = EpochHeader{ConfigEpoch: d.ConfigEpoch, SystemHash: d.SystemHash, ToolCount: d.ToolCount}
			ok = true
		}
	}
	return h, ok
}

func RebuildFromHeader(snap *RequestHeaderSnapshot) (systemPrompt string, toolSchemas json.RawMessage, err error) {
	if snap == nil {
		return "", nil, nil
	}
	return snap.SystemPrompt, snap.ToolSchemas, nil
}

func (s *RequestHeaderSnapshot) SnapshotHash() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%d", s.Header.ConfigEpoch))
	sb.WriteString("|")
	sb.WriteString(s.Header.SystemHash)
	sb.WriteString("|")
	sb.WriteString(fmt.Sprintf("%d", s.Header.ToolCount))
	sb.WriteString("|")
	sb.WriteString(s.Context.Provider)
	sb.WriteString("|")
	sb.WriteString(s.Context.Model)
	sb.WriteString("|")
	sb.WriteString(s.SystemPrompt)
	return sb.String()
}
