package tests

import (
	"context"
	"testing"

	"github.com/JopenChen/dsh-go/pkg/agent"
	"github.com/JopenChen/dsh-go/pkg/brand"
)

func TestInitiatorCausalTrace(t *testing.T) {
	ctx := agent.WithInitiator(context.Background(), brand.NewSessionID("agent_x"), "Run")

	weakCtx := context.WithValue(ctx, "somekey", "v")
	ini, err := agent.RequireInitiator(weakCtx)
	if err != nil {
		t.Fatalf("RequireInitiator failed: %v", err)
	}
	if ini.AgentID.Raw() != "agent_x" {
		t.Fatalf("AgentID = %q, want agent_x", ini.AgentID.Raw())
	}
	if ini.Op != "Run" {
		t.Fatalf("Op = %q, want Run", ini.Op)
	}
}

func TestInitiatorUnauthorized(t *testing.T) {
	_, err := agent.RequireInitiator(context.Background())
	if err == nil {
		t.Fatal("call without initiator should error")
	}
	var unauthorized *agent.UnauthorizedInitiatorError
	if !asUnauthorizedInitiator(err, &unauthorized) {
		t.Fatalf("should be UnauthorizedInitiatorError, got %T", err)
	}
}

func TestInitiatorWithout(t *testing.T) {
	withIni := agent.WithInitiator(context.Background(), brand.NewSessionID("a"), "op")
	cleared := agent.WithoutInitiator(withIni)

	if _, err := agent.RequireInitiator(cleared); err == nil {
		t.Fatal("initiator should be cleared after withoutInitiator")
	}
}

func TestInitiatorMustPanic(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("MustInitiator without initiator should panic")
		}
	}()
	_ = agent.MustInitiator(context.Background())
}

func asUnauthorizedInitiator(err error, target **agent.UnauthorizedInitiatorError) bool {
	e, ok := err.(*agent.UnauthorizedInitiatorError)
	if ok {
		*target = e
	}
	return ok
}
