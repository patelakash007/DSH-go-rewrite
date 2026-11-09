package compaction

import (
	"errors"

	"github.com/JopenChen/dsh-go/pkg/session"
)

var ErrUnbalanced = errors.New("compaction: unbalanced tool pairing")

func toolCallDelta(ev session.SessionEvent) int {
	switch ev.Type {
	case session.EventAssistantMessage:
		if d, ok := ev.Data.(session.AssistantMessageData); ok {
			return len(d.ToolCallIDs)
		}
	case session.EventToolResult:
		return -1
	}
	return 0
}

func BalancedCuts(events []session.SessionEvent) ([]bool, error) {
	cuts := make([]bool, len(events)+1)
	inProgress := 0
	cuts[0] = true
	for i, ev := range events {
		inProgress += toolCallDelta(ev)
		if inProgress < 0 {
			return nil, ErrUnbalanced
		}
		cuts[i+1] = inProgress == 0
	}
	return cuts, nil
}

func IsBalancedAt(events []session.SessionEvent, cut int) (bool, error) {
	cuts, err := BalancedCuts(events)
	if err != nil {
		return false, err
	}
	if cut < 0 || cut >= len(cuts) {
		return false, ErrUnbalanced
	}
	return cuts[cut], nil
}

func NearestBalancedFrom(events []session.SessionEvent, start int) int {
	cuts, err := BalancedCuts(events)
	if err != nil {
		return -1
	}
	for i := start; i < len(cuts); i++ {
		if cuts[i] {
			return i
		}
	}
	return -1
}
