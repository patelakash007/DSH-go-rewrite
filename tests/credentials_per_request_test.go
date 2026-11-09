package tests

import (
	"context"
	"strings"
	"testing"

	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/credentials"
	"github.com/JopenChen/dsh-go/pkg/storage"
)

func TestCredentialsPerRequestResolve(t *testing.T) {
	store := credentials.NewMemoryStore()
	ctx := context.Background()

	apiKey := brand.NewCredentialRef("OPENAI_API_KEY")

	if _, ok := store.Resolve(ctx, apiKey); ok {
		t.Fatal("should not resolve when unset")
	}

	if err := store.Set(ctx, apiKey, "sk-abc"); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	if v, ok := store.Resolve(ctx, apiKey); !ok || v != "sk-abc" {
		t.Fatalf("first round resolve should see new value: %q ok=%v", v, ok)
	}

	if err := store.Set(ctx, apiKey, "sk-new"); err != nil {
		t.Fatalf("second Set failed: %v", err)
	}
	if v, ok := store.Resolve(ctx, apiKey); !ok || v != "sk-new" {
		t.Fatalf("resolve after modification should see new value: %q ok=%v", v, ok)
	}

	if err := store.Unset(ctx, apiKey); err != nil {
		t.Fatalf("Unset failed: %v", err)
	}
	if _, ok := store.Resolve(ctx, apiKey); ok {
		t.Fatal("should not resolve to value after Unset")
	}
}

func TestCredentialsDescribeRedactsValue(t *testing.T) {
	store := credentials.NewMemoryStore()
	ctx := context.Background()

	_ = store.Set(ctx, brand.NewCredentialRef("OPENAI_API_KEY"), "sk-super-secret")
	_ = store.Set(ctx, brand.NewCredentialRef("ANTHROPIC_KEY"), "sk-anthropic")

	infos := store.Describe(ctx)
	if len(infos) != 2 {
		t.Fatalf("describe should contain 2 items: %d", len(infos))
	}

	joined := strings.Join(infosToStrings(infos), "|")
	if strings.Contains(joined, "sk-super-secret") || strings.Contains(joined, "sk-anthropic") {
		t.Fatalf("describe leaks plaintext: %s", joined)
	}

	if infos[0].Ref.Raw() != "ANTHROPIC_KEY" || !infos[0].HasValue {
		t.Fatalf("describe ordering/flags unexpected: %+v", infos)
	}
}

func infosToStrings(infos []credentials.CredentialInfo) []string {
	out := make([]string, 0, len(infos))
	for _, i := range infos {
		out = append(out, i.Ref.Raw()+":"+boolStr(i.HasValue))
	}
	return out
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func TestCredentialsRecordModifyCAS(t *testing.T) {

	sk, err := storage.NewSQLiteKV(t.TempDir() + "/cred.db")
	if err != nil {
		t.Fatalf("NewSQLiteKV failed: %v", err)
	}
	defer sk.Close()

	storeA := credentials.NewStore(sk)
	storeB := credentials.NewStore(sk)
	ctx := context.Background()

	ref := brand.NewCredentialRef("AUTH_TOKEN")
	if err := storeA.Set(ctx, ref, "v1"); err != nil {
		t.Fatalf("storeA Set failed: %v", err)
	}

	_ = storeA.Describe(ctx)
	_ = storeB.Describe(ctx)

	if err := storeB.Set(ctx, ref, "v2"); err != nil {
		t.Fatalf("storeB Set failed: %v", err)
	}

	if err := storeA.Set(ctx, ref, "v3"); err != nil {
		t.Fatalf("storeA write again failed: %v", err)
	}
	if v, ok := storeA.Resolve(ctx, ref); !ok || v != "v3" {
		t.Fatalf("final value should be v3: %q ok=%v", v, ok)
	}
}

func TestAuthorizationFlowRegisterBegin(t *testing.T) {
	svc := credentials.NewAuthorizationService()
	ctx := context.Background()

	flow := &credentials.Flow{
		ID:   credentials.FlowID(brand.NewCredentialRef("github_oauth")),
		Name: "GitHub OAuth",
		Begin: func(ctx context.Context) (string, error) {
			return "https://github.com/login/oauth/authorize?state=xyz", nil
		},
		Cancel: func(ctx context.Context) error { return nil },
	}
	if err := svc.Register(flow); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	if got := svc.List(); len(got) != 1 {
		t.Fatalf("List should contain 1 flow: %d", len(got))
	}

	hint, err := svc.Begin(ctx, flow.ID)
	if err != nil {
		t.Fatalf("Begin failed: %v", err)
	}
	if !strings.HasPrefix(hint, "https://github.com") {
		t.Fatalf("Begin hint unexpected: %q", hint)
	}

	if err := svc.Cancel(ctx, flow.ID); err != nil {
		t.Fatalf("Cancel failed: %v", err)
	}

	if _, err := svc.Begin(ctx, credentials.FlowID(brand.NewCredentialRef("nope"))); err == nil {
		t.Fatal("Begin for unregistered flow should return error")
	}
}
