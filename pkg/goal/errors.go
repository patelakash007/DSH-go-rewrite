//

//   - types.ts  `GoalPhase = 'active'|'paused'|'blocked'|'complete'`；`GoalBlockReason{code,message}`；

//

package goal

import "fmt"

type ErrorCode string

const (
	ErrorAgentNotLive       ErrorCode = "GOAL_AGENT_NOT_LIVE"
	ErrorNotFound           ErrorCode = "GOAL_NOT_FOUND"
	ErrorAlreadyExists      ErrorCode = "GOAL_ALREADY_EXISTS"
	ErrorStaleRevision      ErrorCode = "GOAL_STALE_REVISION"
	ErrorInvalidObjective   ErrorCode = "GOAL_INVALID_OBJECTIVE"
	ErrorInvalidMaxRounds   ErrorCode = "GOAL_INVALID_MAX_ROUNDS"
	ErrorInvalidBlockReason ErrorCode = "GOAL_INVALID_BLOCK_REASON"
	ErrorInvalidEdit        ErrorCode = "GOAL_INVALID_EDIT"
	ErrorInvalidTransition  ErrorCode = "GOAL_INVALID_TRANSITION"
)

type GoalError struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
	Cause   error     `json:"-"`
}

func (e *GoalError) Error() string {
	return fmt.Sprintf("goal error [%s]: %s", e.Code, e.Message)
}

func (e *GoalError) Unwrap() error { return e.Cause }

func NewGoalError(code ErrorCode, message string, cause error) *GoalError {
	return &GoalError{Code: code, Message: message, Cause: cause}
}

func FromError(err error) *GoalError {
	if err == nil {
		return nil
	}
	if ge, ok := err.(*GoalError); ok {
		return ge
	}
	return NewGoalError(ErrorCode("unknown"), err.Error(), err)
}

type GoalBlockReason struct {
	Code string `json:"code"`

	Message string `json:"message"`
}

func NewBlockReason(code, message string) (GoalBlockReason, bool) {
	if code == "" || message == "" {
		return GoalBlockReason{}, false
	}
	return GoalBlockReason{Code: code, Message: message}, true
}
