//

//

package agent

import (
	"context"
	"fmt"

	"github.com/JopenChen/dsh-go/pkg/brand"
)

type initiatorKey struct{}

type Initiator struct {
	AgentID brand.SessionID

	Op string
}

type UnauthorizedInitiatorError struct {
	Op string
}

func (e *UnauthorizedInitiatorError) Error() string {
	return fmt.Sprintf("agent: unauthorized initiator: operation %q called without withInitiator context", e.Op)
}

func WithInitiator(ctx context.Context, agentID brand.SessionID, op string) context.Context {
	return context.WithValue(ctx, initiatorKey{}, &Initiator{AgentID: agentID, Op: op})
}

func RequireInitiator(ctx context.Context) (*Initiator, error) {
	ini, ok := ctx.Value(initiatorKey{}).(*Initiator)
	if !ok || ini == nil {
		return nil, &UnauthorizedInitiatorError{Op: "unknown"}
	}
	return ini, nil
}

func WithoutInitiator(ctx context.Context) context.Context {
	return context.WithValue(ctx, initiatorKey{}, nil)
}

func MustInitiator(ctx context.Context) *Initiator {
	ini, err := RequireInitiator(ctx)
	if err != nil {
		panic(err)
	}
	return ini
}
