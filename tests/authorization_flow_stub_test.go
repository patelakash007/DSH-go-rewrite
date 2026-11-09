//

package tests

import (
	"context"
	"testing"

	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/credentials"
)

func TestAuthFlowBeginCompleteResolve(t *testing.T) {
	ctx := context.Background()
	store := credentials.NewMemoryStore()
	auth := credentials.NewAuthorizationService()
	svc := credentials.NewAuthService(auth, store)

	flowID := brand.NewCredentialRef("GITHUB_TOKEN")

	if err := auth.Register(&credentials.Flow{
		ID:   flowID,
		Name: "github",
		Begin: func(ctx context.Context) (string, error) {
			return "https://example.com/authorize?state=stub", nil
		},
	}); err != nil {
		t.Fatal(err)
	}

	found := false
	for _, f := range auth.List() {
		if f.ID == flowID {
			found = true
		}
	}
	if !found {
		t.Fatal("list flows should contain github")
	}

	// begin → token。
	token, err := svc.Begin(ctx, flowID)
	if err != nil {
		t.Fatal(err)
	}
	if token == "" {
		t.Fatal("begin should return non-empty token")
	}

	if err := svc.Complete(ctx, token, "ghp_stub_secret"); err != nil {
		t.Fatal(err)
	}

	if sess, ok := svc.Status(token); !ok || sess.Status != credentials.AuthCompleted {
		t.Fatalf("session should be completed, got %+v ok=%v", sess, ok)
	}

	if v, ok := store.Resolve(ctx, flowID); !ok || v != "ghp_stub_secret" {
		t.Fatalf("Resolve should get callback credential, got %q ok=%v", v, ok)
	}
}

func TestAuthCancel(t *testing.T) {
	ctx := context.Background()
	store := credentials.NewMemoryStore()
	auth := credentials.NewAuthorizationService()
	svc := credentials.NewAuthService(auth, store)

	flowID := brand.NewCredentialRef("SLACK_TOKEN")
	_ = auth.Register(&credentials.Flow{ID: flowID, Name: "slack",
		Begin: func(context.Context) (string, error) { return "u", nil }})

	token, _ := svc.Begin(ctx, flowID)
	if err := svc.Cancel(ctx, token); err != nil {
		t.Fatal(err)
	}
	sess, _ := svc.Status(token)
	if sess.Status != credentials.AuthCancelled {
		t.Fatalf("state should be cancelled, got %s", sess.Status)
	}

	if err := svc.Complete(ctx, token, "x"); err == nil {
		t.Fatal("cancelled session should not complete again")
	}
}

func TestAuthDoubleCompleteRejected(t *testing.T) {
	ctx := context.Background()
	store := credentials.NewMemoryStore()
	auth := credentials.NewAuthorizationService()
	svc := credentials.NewAuthService(auth, store)

	flowID := brand.NewCredentialRef("A_KEY")
	_ = auth.Register(&credentials.Flow{ID: flowID, Name: "a",
		Begin: func(context.Context) (string, error) { return "u", nil }})

	token, _ := svc.Begin(ctx, flowID)
	if err := svc.Complete(ctx, token, "v1"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Complete(ctx, token, "v2"); err == nil {
		t.Fatal("completed session should be rejected on second complete")
	}

	if v, ok := store.Resolve(ctx, flowID); !ok || v != "v1" {
		t.Fatalf("credential should be initial value v1, got %q", v)
	}
}

func TestAuthMissingToken(t *testing.T) {
	ctx := context.Background()
	store := credentials.NewMemoryStore()
	svc := credentials.NewAuthService(credentials.NewAuthorizationService(), store)
	if _, err := svc.Begin(ctx, brand.NewCredentialRef("NO_FLOW")); err == nil {
		t.Fatal("unregistered flow begin should return error")
	}
	if err := svc.Complete(ctx, "not-exist", "v"); err == nil {
		t.Fatal("unknown token complete should return error")
	}
}
