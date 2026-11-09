//

package tests

import (
	"errors"
	"testing"

	"github.com/JopenChen/dsh-go/pkg/tools"
)

func TestMonotonicGuardFirstSetAccepted(t *testing.T) {
	g := tools.NewMonotonicGuard()
	if err := g.Update(tools.PreAllow); err != nil {
		t.Fatalf("first set should be accepted: %v", err)
	}
	if g.Decision() != tools.PreAllow {
		t.Fatal("decision should be allow")
	}
}

func TestMonotonicGuardCanTighten(t *testing.T) {
	g := tools.NewMonotonicGuard()
	_ = g.Update(tools.PreAllow)
	if err := g.Update(tools.PreDeny); err != nil {
		t.Fatalf("allow→deny tightening should be accepted: %v", err)
	}
	if !g.IsDenied() {
		t.Fatal("guard should be denied after tightening")
	}
}

func TestMonotonicGuardCannotRelax(t *testing.T) {
	g := tools.NewMonotonicGuard()
	_ = g.Update(tools.PreDeny)
	err := g.Update(tools.PreAllow)
	if !errors.Is(err, tools.ErrGuardRelaxed) {
		t.Fatalf("deny→allow relax should be rejected, got %v", err)
	}

	if g.Decision() != tools.PreDeny || !g.IsDenied() {
		t.Fatal("original stricter decision must be preserved")
	}
}

func TestMonotonicGuardAskToDeny(t *testing.T) {
	g := tools.NewMonotonicGuard()
	_ = g.Update(tools.PreAsk)
	if err := g.Update(tools.PreDeny); err != nil {
		t.Fatalf("ask→deny should be accepted: %v", err)
	}
}

func TestMonotonicGuardDenyToAskRejected(t *testing.T) {
	g := tools.NewMonotonicGuard()
	_ = g.Update(tools.PreDeny)
	if err := g.Update(tools.PreAsk); !errors.Is(err, tools.ErrGuardRelaxed) {
		t.Fatalf("deny→ask relax should be rejected, got %v", err)
	}
}

func TestMonotonicGuardInitialIsAsk(t *testing.T) {
	g := tools.NewMonotonicGuard()
	if g.Decision() != tools.PreAsk || g.IsDenied() {
		t.Fatal("fresh guard should be undecided (ask) and not denied")
	}
}
