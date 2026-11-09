//

package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/session"
)

type stubSessionResolver struct {
	sessions map[string]string
	fs       *session.WorkspaceFileResolver
}

func (r *stubSessionResolver) ResolveSession(id brand.SessionID) (string, error) {
	if snap, ok := r.sessions[id.Raw()]; ok {
		return snap, nil
	}
	return "", &session.SessionReferenceError{Code: session.RefCodeSessionNotFound, Msg: "session " + id.Raw() + " not found"}
}

func (r *stubSessionResolver) ResolveFile(p string) (string, error) {
	return r.fs.ResolveFile(p)
}

func (r *stubSessionResolver) WorkspaceRoot() string { return r.fs.WorkspaceRoot() }

func newTestResolver(t *testing.T, sessions map[string]string) *stubSessionResolver {
	t.Helper()
	root := t.TempDir()
	return &stubSessionResolver{
		sessions: sessions,
		fs:       &session.WorkspaceFileResolver{Root: root},
	}
}

func TestSessionReferenceMentionParsing(t *testing.T) {
	text := `Compare implementations in @session/abc-123 and @session/xyz_9, referencing #src/main.go and #docs/readme.md`
	ms := session.ParseMentions(text)
	if len(ms) != 4 {
		t.Fatalf("should parse 4 mentions, got %d: %+v", len(ms), ms)
	}

	want := []session.Mention{
		{Kind: session.MentionSession, Value: "abc-123"},
		{Kind: session.MentionSession, Value: "xyz_9"},
		{Kind: session.MentionFile, Value: "src/main.go"},
		{Kind: session.MentionFile, Value: "docs/readme.md"},
	}
	for i, w := range want {
		if ms[i].Kind != w.Kind || ms[i].Value != w.Value {
			t.Errorf("mention[%d] = %+v, want %+v", i, ms[i], w)
		}
	}
}

func TestSessionReferencePrepareAggregates(t *testing.T) {
	root := t.TempDir()
	mainGo := filepath.Join(root, "src", "main.go")
	if err := os.MkdirAll(filepath.Dir(mainGo), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mainGo, []byte("package main"), 0o644); err != nil {
		t.Fatal(err)
	}

	res := &stubSessionResolver{
		sessions: map[string]string{
			"s1": "session-1 snapshot",
			"s2": "session-2 snapshot",
			"s3": "session-3 snapshot",
		},
		fs: &session.WorkspaceFileResolver{Root: root},
	}
	text := `Combine @session/s1, @session/s2 and @session/s3, then review #src/main.go`
	prepared, err := session.Prepare(session.PrepareRequest{
		AgentSessionID: brand.NewSessionID("self"),
		Text:           text,
		Resolver:       res,
	})
	if err != nil {
		t.Fatalf("Prepare failed: %v", err)
	}

	if strings.Contains(prepared.Content, "@session/") {
		t.Errorf("Content still contains session mention: %q", prepared.Content)
	}
	if strings.Contains(prepared.Content, "#src/main.go") {
		t.Errorf("Content still contains file mention: %q", prepared.Content)
	}

	for _, want := range []string{"session-1 snapshot", "session-2 snapshot", "session-3 snapshot", "file \"src/main.go\""} {
		if !strings.Contains(prepared.AdditionalContext, want) {
			t.Errorf("additionalContext missing %q\ncontext:\n%s", want, prepared.AdditionalContext)
		}
	}

	if len(prepared.References) != 4 {
		t.Fatalf("References should have 4 items, got %d", len(prepared.References))
	}
}

func TestSessionReferenceSelfReferenceRejected(t *testing.T) {
	res := newTestResolver(t, map[string]string{})
	_, err := session.Prepare(session.PrepareRequest{
		AgentSessionID: brand.NewSessionID("me"),
		Text:           `Look at @session/me`,
		Resolver:       res,
	})
	if session.RefCodeFrom(err) != session.RefCodeSelfReference {
		t.Fatalf("should return SELF_REFERENCE, got: %v", err)
	}
}

func TestSessionReferenceClassificationErrors(t *testing.T) {
	root := t.TempDir()
	res := newTestResolver(t, map[string]string{})
	res.fs.Root = root

	_, err := session.Prepare(session.PrepareRequest{
		Text:     `Read @session/ghost`,
		Resolver: res,
	})
	if session.RefCodeFrom(err) != session.RefCodeSessionNotFound {
		t.Fatalf("should return SESSION_NOT_FOUND, got: %v", err)
	}

	_, err = session.Prepare(session.PrepareRequest{
		Text:     `See #no/such/file.go`,
		Resolver: res,
	})
	if session.RefCodeFrom(err) != session.RefCodeFileNotFound {
		t.Fatalf("should return FILE_NOT_FOUND, got: %v", err)
	}

	_, err = session.Prepare(session.PrepareRequest{
		Text:     `Read #../outside.txt`,
		Resolver: res,
	})
	if session.RefCodeFrom(err) != session.RefCodeFileOutOfWorkspace {
		t.Fatalf("should return FILE_OUT_OF_WORKSPACE, got: %v", err)
	}
}

func TestSessionReferenceTooManyRejected(t *testing.T) {
	res := newTestResolver(t, map[string]string{})
	var sb strings.Builder
	for i := 0; i < 20; i++ {
		sb.WriteString("@session/abc")
		sb.WriteString(string(rune('a' + i)))
		sb.WriteString(" ")
	}
	_, err := session.Prepare(session.PrepareRequest{
		Text:     sb.String(),
		Resolver: res,
	})
	if session.RefCodeFrom(err) != session.RefCodeTooMany {
		t.Fatalf("should return TOO_MANY, got: %v", err)
	}
}

func TestSessionReferenceBudgetExceeded(t *testing.T) {
	res := newTestResolver(t, map[string]string{"big": strings.Repeat("x", 1000)})
	_, err := session.Prepare(session.PrepareRequest{
		Text:            `Look at @session/big`,
		Resolver:        res,
		MaxContextBytes: 100,
	})
	if session.RefCodeFrom(err) != session.RefCodeBudgetExceeded {
		t.Fatalf("should return BUDGET_EXCEEDED, got: %v", err)
	}
}

func TestSessionReferenceWriteAuditData(t *testing.T) {
	res := newTestResolver(t, map[string]string{"s1": "snapshot-1"})
	prepared, err := session.Prepare(session.PrepareRequest{
		Text:     `Combine @session/s1`,
		Resolver: res,
	})
	if err != nil {
		t.Fatal(err)
	}

	refs := make([]any, 0, len(prepared.References))
	for _, r := range prepared.References {
		refs = append(refs, r)
	}
	msg := session.UserMessageData{
		Content: prepared.Content,
		Source:  "reference",
		Refs:    refs,
	}

	if msg.Source != "reference" || msg.Content != prepared.Content {
		t.Fatalf("user/message audit data incomplete: %+v", msg)
	}
}
