package tests

import (
	"testing"

	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/session"
)

func TestSessionHeaderRoundTrip(t *testing.T) {
	h := session.NewSessionHeader(brand.NewSessionID("s1"), "/workspace")
	h.AgentPreset = "coder"

	data, err := h.Marshal()
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	restored, err := session.UnmarshalSessionHeader(data)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if restored.ID.Raw() != "s1" || restored.Cwd != "/workspace" || restored.AgentPreset != "coder" {
		t.Fatalf("restored fields mismatch: %+v", restored)
	}
	if restored.Version != session.SessionFormatVersion {
		t.Fatalf("version = %d, want %d", restored.Version, session.SessionFormatVersion)
	}
	if restored.Origin != session.OriginCreated {
		t.Fatalf("origin = %q, want created", restored.Origin)
	}
	if restored.DelegationDepth != 0 {
		t.Fatalf("initial delegation depth should be 0: %d", restored.DelegationDepth)
	}
}

func TestSessionHeaderUnknownVersionRejected(t *testing.T) {

	data := []byte(`{"version":999,"id":"s_x","createdAt":"2026-08-31T00:00:00Z"}`)

	_, err := session.UnmarshalSessionHeader(data)
	if err == nil {
		t.Fatal("header with unknown version should be rejected")
	}

	var unsupported *session.SessionFormatUnsupportedError
	if !asUnsupported(err, &unsupported) {
		t.Fatalf("should return SessionFormatUnsupportedError, got %T", err)
	}
}

func asUnsupported(err error, target **session.SessionFormatUnsupportedError) bool {
	e, ok := err.(*session.SessionFormatUnsupportedError)
	if ok {
		*target = e
	}
	return ok
}

func TestSessionHeaderForkLineage(t *testing.T) {
	parent := session.NewSessionHeader(brand.NewSessionID("parent_s"), "/ws")

	parent.SeedLength = 42

	child := parent.Fork(brand.NewSessionID("child_s"))
	if child.ParentSession.Raw() != "parent_s" {
		t.Fatalf("child.ParentSession = %q, want parent_s", child.ParentSession.Raw())
	}
	if child.Origin != session.OriginFork {
		t.Fatalf("child.Origin = %q, want fork", child.Origin)
	}
	if child.DelegationDepth != 1 {
		t.Fatalf("child.DelegationDepth = %d, want 1", child.DelegationDepth)
	}
	if child.SeedLength != 0 {
		t.Fatalf("fork child session SeedLength should be 0: %d", child.SeedLength)
	}
	if child.Cwd != "/ws" {
		t.Fatalf("child.Cwd should inherit parent session: %q", child.Cwd)
	}

	child.SeedLength = 7

	grandchild := child.Fork(brand.NewSessionID("gc_s"))
	if grandchild.DelegationDepth != 2 {
		t.Fatalf("grandchild.DelegationDepth = %d, want 2", grandchild.DelegationDepth)
	}
	if grandchild.ParentSession.Raw() != "child_s" {
		t.Fatalf("grandchild.ParentSession = %q, want child_s", grandchild.ParentSession.Raw())
	}
}

func TestSessionHeaderValidate(t *testing.T) {
	bad := &session.SessionHeader{Version: session.SessionFormatVersion}
	if err := bad.Validate(); err == nil {
		t.Fatal("header validation with missing ID should fail")
	}

	good := session.NewSessionHeader(brand.NewSessionID("s1"), "/ws")
	if err := good.Validate(); err != nil {
		t.Fatalf("valid header validation should pass: %v", err)
	}
}
