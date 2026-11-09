//

//

package tools

import (
	"fmt"

	"github.com/JopenChen/dsh-go/pkg/approval"
	"github.com/JopenChen/dsh-go/pkg/waterfall"
)

type PreToolDecision = approval.Decision

const (
	PreAllow = approval.DecideAllow
	PreDeny  = approval.DecideDeny
	PreAsk   = approval.DecideAsk
)

type DecisionFunc func(req *ToolCallRequest) (PreToolDecision, error)

type AskFunc func(req *ToolCallRequest) (bool, error)

const denyReasonKey = "preDecision.reason"

func DecisionFromApproval(decide func(req *ToolCallRequest) (approval.Decision, error)) DecisionFunc {
	return func(req *ToolCallRequest) (PreToolDecision, error) {
		return decide(req)
	}
}

//

func PreDecisionMiddleware(decide DecisionFunc, ask AskFunc) waterfall.Handler[ExecContext] {
	return func(ec *ExecContext, next waterfall.NextFunc) error {
		if ec == nil || ec.Request == nil || decide == nil {
			return next()
		}
		decision, err := decide(ec.Request)
		if err != nil {
			ec.Denied = true
			ec.Meta[denyReasonKey] = fmt.Sprintf("decision error: %v", err)
			return next()
		}
		switch decision {
		case PreAllow:

			return next()
		case PreDeny:
			ec.Denied = true
			ec.Meta[denyReasonKey] = "denied by pre-tool decision"
			return next()
		case PreAsk:
			if ask == nil {
				ec.Denied = true
				ec.Meta[denyReasonKey] = "ask requested but no ask handler"
				return next()
			}
			allowed, aerr := ask(ec.Request)
			if aerr != nil || !allowed {
				ec.Denied = true
				ec.Meta[denyReasonKey] = "ask denied by user"
				return next()
			}

			ec.Meta["allowedOnce"] = ec.Request.CallID.Raw()
			return next()
		default:
			ec.Denied = true
			ec.Meta[denyReasonKey] = fmt.Sprintf("unknown decision %d", decision)
			return next()
		}
	}
}

func AskFromApproval(evaluate func(req *ToolCallRequest) (approval.Decision, error)) AskFunc {
	return func(req *ToolCallRequest) (bool, error) {
		dec, err := evaluate(req)
		if err != nil {
			return false, err
		}
		return dec == approval.DecideAllow, nil
	}
}
