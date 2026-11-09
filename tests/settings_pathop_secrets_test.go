package tests

import (
	"strings"
	"testing"

	"github.com/JopenChen/dsh-go/pkg/settings"
)

func TestSettingsPathOpSecrets(t *testing.T) {
	ns := settings.NewNamespace("dsh")
	ns.MarkSecret("credentials.api_key")
	ns.MarkSecret("credentials.secret")

	s := settings.NewSettingsScope(ns)
	_ = s.ApplyHost("sandbox.mode", "read-only")
	_ = s.ApplyHost("credentials.api_key", "sk-secret-123")

	err := s.Update(0, map[settings.Path]settings.PathOp{
		"sandbox.mode":       settings.NewSetOp("workspace-write"),
		"credentials.secret": settings.NewSetOp("supersecret"),
		"unused.path":        settings.NewUnsetOp(),
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	desc := s.Describe(true)
	apiKey := findPath(desc, "credentials.api_key")
	secret := findPath(desc, "credentials.secret")
	if apiKey == nil || secret == nil {
		t.Fatalf("secret path should exist: %+v", desc)
	}
	if apiKey.Value != nil {
		t.Fatalf("secret api_key should be masked: %+v", apiKey)
	}
	if secret.Value != nil {
		t.Fatalf("secret secret should be masked: %+v", secret)
	}
	if !apiKey.Secret || !secret.Secret {
		t.Fatal("secret flag should be true")
	}

	descOpen := s.Describe(false)
	if p := findPath(descOpen, "credentials.secret"); p == nil || p.Value != "supersecret" {
		t.Fatalf("plaintext should be visible when unmasked: %+v", p)
	}
}

func findPath(list []settings.DescribedSetting, p settings.Path) *settings.DescribedSetting {
	for i := range list {
		if list[i].Path == p {
			return &list[i]
		}
	}
	return nil
}

func TestSettingsCASConcurrent(t *testing.T) {
	ns := settings.NewNamespace("dsh")
	s := settings.NewSettingsScope(ns)

	if _, err := 0, func() error {
		return s.Replace(0, map[settings.Path]any{"a": 1})
	}(); err != nil {
		t.Fatalf("first replace failed: %v", err)
	}
	rev1 := s.Revision()
	if rev1 != 1 {
		t.Fatalf("revision after first replace = %d, want 1", rev1)
	}

	if err := s.Replace(rev1, map[settings.Path]any{"b": 2}); err != nil {
		t.Fatalf("second replace failed: %v", err)
	}
	if s.Revision() != 2 {
		t.Fatalf("revision after second replace = %d, want 2", s.Revision())
	}

	err := s.Replace(rev1, map[settings.Path]any{"c": 3})
	if !settings.IsRevisionMismatch(err) {
		t.Fatalf("stale revision write should trigger CAS error, got %v", err)
	}
	var mismatch *settings.ErrRevisionMismatch
	if !asRevMismatch(err, &mismatch) {
		t.Fatalf("error type should be *ErrRevisionMismatch: %T", err)
	}
	if mismatch.Expected != rev1 || mismatch.Actual != 2 {
		t.Fatalf("CAS fields unexpected: %+v", mismatch)
	}
}

func asRevMismatch(err error, target **settings.ErrRevisionMismatch) bool {
	e, ok := err.(*settings.ErrRevisionMismatch)
	if ok {
		*target = e
	}
	return ok
}

func TestSettingsNearestWins(t *testing.T) {
	ns := settings.NewNamespace("dsh")
	s := settings.NewSettingsScope(ns)
	_ = s.ApplyHost("sandbox.mode", "read-only")

	if v, ok := s.Get("sandbox.mode"); !ok || !strings.Contains(string(v), "read-only") {
		t.Fatalf("should read host default when uncovered: %q ok=%v", v, ok)
	}

	if err := s.Update(0, map[settings.Path]settings.PathOp{
		"sandbox.mode": settings.NewSetOp("danger-full-access"),
	}); err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if v, ok := s.Get("sandbox.mode"); !ok || !strings.Contains(string(v), "danger-full-access") {
		t.Fatalf("should read new value after session override: %q ok=%v", v, ok)
	}

	desc := s.Describe(true)
	if p := findPath(desc, "sandbox.mode"); p == nil || p.Op != "set" {
		t.Fatalf("op after session override should be set: %+v", p)
	}
}
