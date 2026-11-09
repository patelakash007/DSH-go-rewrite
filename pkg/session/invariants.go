//

//

package session

import (
	"fmt"
	"sort"

	"github.com/JopenChen/dsh-go/pkg/brand"
)

// ============================================================================

// ============================================================================

const AppendOnlyMarker = "SessionLog exposes a single write path: Append(EventData)"

//

func AppendOnly(sl *SessionLog) error {
	if sl == nil {
		return fmt.Errorf("session: append-only check on nil log")
	}
	return nil
}

// ============================================================================

// ============================================================================

type InvariantCode string

const (
	InvSeqContinuous    InvariantCode = "seq_continuous"
	InvTimeMonotonic    InvariantCode = "time_monotonic"
	InvTurnPaired       InvariantCode = "turn_start_end_paired"
	InvStepPaired       InvariantCode = "step_start_end_paired"
	InvApprovalPaired   InvariantCode = "approval_asked_decided_paired"
	InvGoalCAS          InvariantCode = "goal_revision_cas"
	InvToolCallResult   InvariantCode = "tool_call_result_paired"
	InvFormatConsistent InvariantCode = "persistence_format_consistent"
)

type InvariantFailure struct {
	Code    InvariantCode
	Message string
}

func (f InvariantFailure) Error() string {
	return fmt.Sprintf("invariant [%s]: %s", f.Code, f.Message)
}

func VerifyInvariants(events []SessionEvent) []error {
	var fails []error
	fails = append(fails, checkSeqContinuous(events)...)
	fails = append(fails, checkTimeMonotonic(events)...)
	fails = append(fails, checkTurnPaired(events)...)
	fails = append(fails, checkStepPaired(events)...)
	fails = append(fails, checkApprovalPaired(events)...)
	fails = append(fails, checkGoalCAS(events)...)
	fails = append(fails, checkToolCallResult(events)...)
	fails = append(fails, checkFormatConsistent(events)...)
	return fails
}

func checkSeqContinuous(events []SessionEvent) []error {
	seq := uint64(0)
	var fails []error
	seen := map[EventType]bool{}
	_ = seen
	for _, e := range events {
		if e.SurfaceOp != nil {
			continue
		}
		if e.Type == EventSurfaceReplace {
			continue
		}
		seq++
		if e.Seq != seq {
			fails = append(fails, &InvariantFailure{Code: InvSeqContinuous,
				Message: fmt.Sprintf("seq gap at %d (got %d)", seq, e.Seq)})
		}
	}
	return fails
}

func checkTimeMonotonic(events []SessionEvent) []error {
	var fails []error
	var last int64 = -1
	for i, e := range events {
		if i == 0 {
			last = e.Time.UnixNano()
			continue
		}
		if e.Time.UnixNano() < last {
			fails = append(fails, &InvariantFailure{Code: InvTimeMonotonic,
				Message: fmt.Sprintf("time rewind at seq %d", e.Seq)})
		}
		last = e.Time.UnixNano()
	}
	return fails
}

func checkTurnPaired(events []SessionEvent) []error {
	var fails []error
	depth := 0
	for _, e := range events {
		switch e.Type {
		case EventTurnStart:
			depth++
		case EventTurnEnd:
			depth--
		}
		if depth < 0 {
			fails = append(fails, &InvariantFailure{Code: InvTurnPaired,
				Message: "turn/end without matching turn/start"})
			depth = 0
		}
	}
	if depth != 0 {
		fails = append(fails, &InvariantFailure{Code: InvTurnPaired, Message: "unbalanced turn start/end"})
	}
	return fails
}

func checkStepPaired(events []SessionEvent) []error {
	var fails []error
	depth := 0
	for _, e := range events {
		switch e.Type {
		case EventStepStart:
			depth++
		case EventStepEnd:
			depth--
		}
		if depth < 0 {
			fails = append(fails, &InvariantFailure{Code: InvStepPaired, Message: "step/end without start"})
			depth = 0
		}
	}
	if depth != 0 {
		fails = append(fails, &InvariantFailure{Code: InvStepPaired, Message: "unbalanced step start/end"})
	}
	return fails
}

func checkApprovalPaired(events []SessionEvent) []error {
	var fails []error
	open := map[string]bool{}
	for _, e := range events {
		switch e.Type {
		case EventApprovalAsked:
			if d, ok := e.Data.(ApprovalRequestIDData); ok {
				open[d.RequestID.Raw()] = true
			}
		case EventApprovalDecided:
			if d, ok := e.Data.(ApprovalDecidedData); ok {
				if !open[d.RequestID.Raw()] {
					fails = append(fails, &InvariantFailure{Code: InvApprovalPaired,
						Message: "approval/decided without asked " + d.RequestID.Raw()})
				}
				delete(open, d.RequestID.Raw())
			}
		}
	}
	if len(open) > 0 {
		fails = append(fails, &InvariantFailure{Code: InvApprovalPaired, Message: "approval/asked left undecided"})
	}
	return fails
}

func checkGoalCAS(events []SessionEvent) []error {
	var fails []error
	rev := map[string]uint64{}
	for _, e := range events {
		if e.Type != EventGoalChange {
			continue
		}
		d, ok := e.Data.(GoalChangeData)
		if !ok {
			continue
		}
		last := rev[d.GoalID]
		if d.Revision != last+1 {
			fails = append(fails, &InvariantFailure{Code: InvGoalCAS,
				Message: fmt.Sprintf("goal %s revision %d, expected %d", d.GoalID, d.Revision, last+1)})
		}
		rev[d.GoalID] = d.Revision
	}
	return fails
}

func checkToolCallResult(events []SessionEvent) []error {
	var fails []error
	state := map[string]bool{}
	for _, e := range events {
		switch e.Type {
		case EventToolCall:
			if d, ok := e.Data.(ToolCallData); ok {
				if _, dup := state[d.CallID.Raw()]; dup {
					fails = append(fails, &InvariantFailure{Code: InvToolCallResult,
						Message: "duplicate tool/call " + d.CallID.Raw()})
				}
				state[d.CallID.Raw()] = false
			}
		case EventToolResult, EventToolError:
			var callID brand.ToolCallID
			if d, ok := e.Data.(ToolResultData); ok {
				callID = d.CallID
			} else if d2, ok := e.Data.(ToolErrorData); ok {
				callID = d2.CallID
			} else {
				continue
			}
			key := callID.Raw()
			done, ok := state[key]
			if !ok || done {
				fails = append(fails, &InvariantFailure{Code: InvToolCallResult,
					Message: "tool/result without open call " + key})
			} else {
				state[key] = true
			}
		}
	}
	return fails
}

func checkFormatConsistent(events []SessionEvent) []error {
	var fails []error
	for _, e := range events {
		if !knownType(e.Type) {
			fails = append(fails, &InvariantFailure{Code: InvFormatConsistent,
				Message: "unknown event type " + string(e.Type)})
		}
	}
	return fails
}

func knownType(t EventType) bool {
	for _, k := range knownSessionEventTypes {
		if k == t {
			return true
		}
	}
	return false
}

var knownSessionEventTypes []EventType

func init() {
	knownSessionEventTypes = make([]EventType, len(AllEventTypes))
	copy(knownSessionEventTypes, AllEventTypes)
	sort.Slice(knownSessionEventTypes, func(i, j int) bool { return knownSessionEventTypes[i] < knownSessionEventTypes[j] })
}
