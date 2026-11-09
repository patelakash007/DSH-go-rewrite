//

//

package agent

import (
	"sync"

	"github.com/JopenChen/dsh-go/pkg/session"
)

type InboxNotifications struct {
	Inserted func(message UserMessage)

	Discarded func(message UserMessage)

	Claimed func(message UserMessage, turn uint64)
}

//

type Inbox struct {
	mu            sync.Mutex
	nextTurn      []UserMessage
	nextStep      []UserMessage
	notifications InboxNotifications
}

func NewInbox(log *session.SessionLog, notifications InboxNotifications) *Inbox {
	inbox := &Inbox{
		notifications: notifications,
	}
	if log != nil {

		for _, ev := range log.Events() {
			if ev.Type == session.EventAgentInboxSpliced {
				if data, ok := ev.Data.(session.InboxSplicedData); ok {
					inbox.apply(data)
				}
			}
		}
	}
	return inbox
}

func (inbox *Inbox) NextTurn() []UserMessage {
	inbox.mu.Lock()
	defer inbox.mu.Unlock()
	result := make([]UserMessage, len(inbox.nextTurn))
	copy(result, inbox.nextTurn)
	return result
}

func (inbox *Inbox) NextStep() []UserMessage {
	inbox.mu.Lock()
	defer inbox.mu.Unlock()
	result := make([]UserMessage, len(inbox.nextStep))
	copy(result, inbox.nextStep)
	return result
}

func (inbox *Inbox) HasPending() bool {
	inbox.mu.Lock()
	defer inbox.mu.Unlock()
	return len(inbox.nextTurn) > 0 || len(inbox.nextStep) > 0
}

func (inbox *Inbox) Clear(log *session.SessionLog) {
	inbox.splice(log, session.InboxNextStep, 0, len(inbox.nextStep), nil, "canceled")
	inbox.splice(log, session.InboxNextTurn, 0, len(inbox.nextTurn), nil, "canceled")
}

//

func (inbox *Inbox) Claim(log *session.SessionLog, target session.InboxTarget, turn uint64) []UserMessage {
	inbox.mu.Lock()
	claimed := inbox.mutate(session.InboxNextStep, 0, len(inbox.nextStep), nil)
	if target == session.InboxNextTurn {
		claimed = append(claimed, inbox.mutate(session.InboxNextTurn, 0, 1, nil)...)
	}
	inbox.mu.Unlock()

	if inbox.notifications.Claimed != nil {
		for _, msg := range claimed {
			inbox.notifications.Claimed(msg, turn)
		}
	}

	if log != nil {
		if len(claimed) > 0 {

			stepRemoved := len(inbox.NextStep())
			_ = stepRemoved

			_, _ = log.Append(session.InboxSplicedData{
				Target:       session.InboxNextStep,
				Start:        0,
				RemovedCount: len(claimed),
				Inserted:     nil,
			})
		}
	}

	return claimed
}

func (inbox *Inbox) Push(log *session.SessionLog, target session.InboxTarget, message UserMessage) {
	inbox.splice(log, target, -1, 0, []UserMessage{message}, "")
}

func (inbox *Inbox) splice(log *session.SessionLog, target session.InboxTarget, start, removedCount int, inserted []UserMessage, outcome string) {
	inbox.mu.Lock()
	removed := inbox.mutate(target, start, removedCount, inserted)
	inbox.mu.Unlock()

	if inbox.notifications.Discarded != nil {
		for _, msg := range removed {
			inbox.notifications.Discarded(msg)
		}
	}
	if inbox.notifications.Inserted != nil {
		for _, msg := range inserted {
			inbox.notifications.Inserted(msg)
		}
	}

	if log != nil {
		insertedContents := make([]string, len(inserted))
		for i, msg := range inserted {
			insertedContents[i] = msg.Content
		}
		_, _ = log.Append(session.InboxSplicedData{
			Target:       target,
			Start:        start,
			RemovedCount: removedCount,
			Inserted:     insertedContents,
			Outcome:      outcome,
		})
	}
}

func (inbox *Inbox) mutate(target session.InboxTarget, start, removedCount int, inserted []UserMessage) []UserMessage {
	var list *[]UserMessage
	switch target {
	case session.InboxNextTurn:
		list = &inbox.nextTurn
	case session.InboxNextStep:
		list = &inbox.nextStep
	default:
		return nil
	}

	if start == -1 {
		start = len(*list)
	}

	if start < 0 {
		start = 0
	}
	if start > len(*list) {
		start = len(*list)
	}
	if removedCount < 0 {
		removedCount = 0
	}
	if start+removedCount > len(*list) {
		removedCount = len(*list) - start
	}

	removed := make([]UserMessage, removedCount)
	copy(removed, (*list)[start:start+removedCount])

	newList := make([]UserMessage, 0, len(*list)-removedCount+len(inserted))
	newList = append(newList, (*list)[:start]...)
	newList = append(newList, inserted...)
	newList = append(newList, (*list)[start+removedCount:]...)
	*list = newList

	return removed
}

func (inbox *Inbox) apply(data session.InboxSplicedData) {
	inbox.mu.Lock()
	defer inbox.mu.Unlock()

	inserted := make([]UserMessage, len(data.Inserted))
	for i, content := range data.Inserted {
		inserted[i] = UserMessage{Content: content}
	}

	inbox.mutate(data.Target, data.Start, data.RemovedCount, inserted)
}
