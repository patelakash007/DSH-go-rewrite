//

//

package agent

import (
	"strings"

	"github.com/JopenChen/dsh-go/pkg/session"
)

const cancelReasonPrefix = "cancel:"

func RecordCancel(sl *session.SessionLog, cause CancelCause) error {
	_, err := sl.Append(session.TurnStoppingData{Reason: cancelReasonPrefix + string(cause)})
	return err
}

func ExtractCancelCause(events []session.SessionEvent) (CancelCause, bool) {
	var found CancelCause
	ok := false
	for _, ev := range events {
		if ev.Type != session.EventTurnStopping {
			continue
		}
		d, isData := ev.Data.(session.TurnStoppingData)
		if !isData || !strings.HasPrefix(d.Reason, cancelReasonPrefix) {
			continue
		}
		found = CancelCause(strings.TrimPrefix(d.Reason, cancelReasonPrefix))
		ok = true
	}
	return found, ok
}

func AllCancelCauses() []CancelCause {
	return []CancelCause{CancelUser, CancelParent, CancelHook, CancelDisposed, CancelLegacy}
}
