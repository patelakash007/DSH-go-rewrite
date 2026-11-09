package tests

import (
	"strings"
	"testing"

	"github.com/JopenChen/dsh-go/pkg/scope"
)

func TestScopeNearestWins(t *testing.T) {

	host := scope.NewLayer[string](scope.Key("host"))
	host.Register("bash", "host/bash:full", 0)

	session := scope.NewLayer[string](scope.Key("session:s1"))
	session.Register("bash", "session/bash:restricted", 0)

	layers := scope.NewScopedLayers[string]().
		Push(host).
		Push(session)

	got, ok := layers.Get("bash")
	if !ok {
		t.Fatal("bash should be resolvable")
	}
	if got != "session/bash:restricted" {
		t.Fatalf("nearest-scope-wins failed: got %q", got)
	}

	host.Register("ls", "host/ls", 0)
	got, ok = layers.Get("ls")
	if !ok || got != "host/ls" {
		t.Fatalf("host-only key resolution failed: got %q ok=%v", got, ok)
	}

	if _, ok := layers.Get("nope"); ok {
		t.Fatal("non-existent key should not resolve successfully")
	}
}

func TestScopeRankTiebreak(t *testing.T) {
	layer := scope.NewLayer[int](scope.Key("session:s1"))

	layer.Register("toolA", 1, 0)
	layer.Register("toolA", 2, 0)

	layers := scope.NewScopedLayers[int]().Push(layer)
	if v, ok := layers.Get("toolA"); !ok || v != 2 {
		t.Fatalf("same rank should overwrite with later registration: got %d ok=%v", v, ok)
	}

	layer.Register("toolB", 10, 1)
	layer.Register("toolB", 20, 0)
	if v, ok := layers.Get("toolB"); !ok || v != 10 {
		t.Fatalf("higher rank wins: got %d ok=%v", v, ok)
	}
}

func TestScopeAnonymousNotConflict(t *testing.T) {
	layer := scope.NewLayer[string](scope.Key("host"))

	layer.Register("bash", "named-bash", 0)

	layer.Register("", "anonymous-default", 0)

	layers := scope.NewScopedLayers[string]().Push(layer)

	if v, ok := layers.Get("bash"); !ok || v != "named-bash" {
		t.Fatalf("named lookup should hit named entry: got %q ok=%v", v, ok)
	}

	if _, ok := layers.Get(""); ok {
		t.Fatal("empty string lookup should not hit")
	}

	merged := layers.Merge()
	hasAnon, hasNamed := false, false
	for _, e := range merged {
		if e.Name == "" {
			hasAnon = true
		}
		if e.Name == "bash" {
			hasNamed = true
		}
	}
	if !hasAnon || !hasNamed {
		t.Fatalf("anonymous and named entries should both be retained in Merge result: %+v", merged)
	}
}

func TestScopeMergeDeterministic(t *testing.T) {
	host := scope.NewLayer[string](scope.Key("host"))
	host.Register("zebra", "z", 0)
	host.Register("alpha", "a", 0)
	session := scope.NewLayer[string](scope.Key("session"))
	session.Register("mike", "m", 0)

	layers := scope.NewScopedLayers[string]().Push(host).Push(session)

	first := layers.Merge()
	second := layers.Merge()

	if len(first) != len(second) {
		t.Fatalf("two Merge lengths mismatch")
	}
	for i := range first {
		if first[i].Name != second[i].Name || first[i].Value != second[i].Value {
			t.Fatalf("Merge output non-deterministic: %+v vs %+v", first, second)
		}
	}

	var names []string
	for _, e := range first {
		if e.Name != "" {
			names = append(names, e.Name)
		}
	}
	joined := strings.Join(names, ",")
	if joined != "alpha,mike,zebra" {
		t.Fatalf("named entries should be lexicographical: %q", joined)
	}
}
