//

//

package agent

import (
	"github.com/JopenChen/dsh-go/pkg/session"
)

type ConsumedWork struct {
	End *session.SessionEvent

	DroppedUnrun bool
}

//

func accountsForClaim(reason session.TurnEndReason) bool {
	switch reason {
	case session.ReasonCompleted, session.ReasonFinished:
		return false
	case session.ReasonBlocked, session.ReasonAborted, session.ReasonInterrupted, session.ReasonError:
		return true
	default:

		return true
	}
}

func FoldConsumedWork(events []session.SessionEvent) ConsumedWork {
	stepped := make(map[uint64]struct{})
	claimed := make(map[uint64]struct{})
	var open uint64
	hasOpen := false
	var end *session.SessionEvent
	droppedUnrun := false

	for i := range events {
		ev := &events[i]
		switch ev.Type {
		case session.EventTurnStart:
			if d, ok := ev.Data.(session.TurnStartData); ok {
				open = d.Turn
				hasOpen = true
			}
		case session.EventStepStart:
			if d, ok := ev.Data.(session.StepStartData); ok {
				stepped[d.Turn] = struct{}{}
			}
		case session.EventAgentInboxSpliced:
			d, ok := ev.Data.(session.InboxSplicedData)
			if !ok || d.RemovedCount == 0 {
				continue
			}
			if d.Outcome == "canceled" {

				droppedUnrun = droppedUnrun || len(d.Inserted) == 0
			} else if hasOpen {

				claimed[open] = struct{}{}
			}
		case session.EventTurnEnd:
			d, ok := ev.Data.(session.TurnEndData)
			if !ok {
				continue
			}
			hasOpen = false
			_, didStep := stepped[d.Turn]
			_, didClaim := claimed[d.Turn]
			if didStep || (didClaim && accountsForClaim(d.Reason)) {
				cp := *ev
				end = &cp

				droppedUnrun = false
			}
			delete(stepped, d.Turn)
			delete(claimed, d.Turn)
		}
	}

	return ConsumedWork{End: end, DroppedUnrun: droppedUnrun}
}
