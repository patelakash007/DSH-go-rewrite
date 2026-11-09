//

package tests

import (
	"testing"

	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/sandbox"
	"github.com/JopenChen/dsh-go/pkg/session"
)

func TestSandboxModeEventType(t *testing.T) {
	if session.EventSandboxMode != "sandbox/mode" {
		t.Fatalf("event type should be sandbox/mode, got %s", session.EventSandboxMode)
	}
	data := session.SandboxModeData{Mode: string(sandbox.ModeWorkspaceWrite)}
	if data.EventType() != session.EventSandboxMode {
		t.Fatal("SandboxModeData.EventType() should return EventSandboxMode")
	}
}

func TestSandboxModeRoundTrip(t *testing.T) {
	sl := session.NewSessionLog(brand.NewSessionID("sbox-test"))
	_, err := sl.Append(session.SandboxModeData{Mode: string(sandbox.ModeReadOnly)})
	if err != nil {
		t.Fatalf("append sandbox/mode failed: %v", err)
	}
	evs := sl.Events()
	if len(evs) != 1 {
		t.Fatalf("should have 1 event, got %d", len(evs))
	}
	if evs[0].Type != session.EventSandboxMode {
		t.Fatalf("event type should be sandbox/mode, got %s", evs[0].Type)
	}
	data, ok := evs[0].Data.(session.SandboxModeData)
	if !ok {
		t.Fatal("event data should be SandboxModeData")
	}
	if data.Mode != string(sandbox.ModeReadOnly) {
		t.Fatalf("mode should be read-only, got %s", data.Mode)
	}
}

func TestEffectiveSandboxModeFold(t *testing.T) {
	sl := session.NewSessionLog(brand.NewSessionID("sbox-fold"))
	sl.Append(session.SandboxModeData{Mode: string(sandbox.ModeReadOnly)})
	sl.Append(session.UserMessageData{Content: "hello"})
	sl.Append(session.SandboxModeData{Mode: string(sandbox.ModeWorkspaceWrite)})
	sl.Append(session.AssistantMessageData{Content: "hi"})

	mode, ok := sandbox.EffectiveSandboxMode(sl.Events())
	if !ok {
		t.Fatal("should have sandbox/mode event")
	}
	if mode != sandbox.ModeWorkspaceWrite {
		t.Fatalf("should take latest workspace-write, got %s", mode)
	}
}

func TestEffectiveSandboxModeNoEvent(t *testing.T) {
	sl := session.NewSessionLog(brand.NewSessionID("sbox-none"))
	sl.Append(session.UserMessageData{Content: "hello"})

	_, ok := sandbox.EffectiveSandboxMode(sl.Events())
	if ok {
		t.Fatal("should return ok=false when no sandbox/mode event")
	}
}

func TestEffectiveSandboxModeEmpty(t *testing.T) {
	_, ok := sandbox.EffectiveSandboxMode(nil)
	if ok {
		t.Fatal("empty event list should return ok=false")
	}
}

func TestSetSandboxMode(t *testing.T) {
	sl := session.NewSessionLog(brand.NewSessionID("sbox-set"))
	err := sandbox.SetSandboxMode(sl, sandbox.ModeDangerFullAccess)
	if err != nil {
		t.Fatalf("SetSandboxMode failed: %v", err)
	}
	evs := sl.Events()
	if len(evs) != 1 {
		t.Fatalf("should have 1 event, got %d", len(evs))
	}
	if evs[0].Type != session.EventSandboxMode {
		t.Fatalf("event type should be sandbox/mode, got %s", evs[0].Type)
	}
	data := evs[0].Data.(session.SandboxModeData)
	if data.Mode != string(sandbox.ModeDangerFullAccess) {
		t.Fatalf("mode should be danger-full-access, got %s", data.Mode)
	}
}

func TestSetSandboxModeDelegation(t *testing.T) {
	sl := session.NewSessionLog(brand.NewSessionID("sbox-deleg"))
	err := sandbox.SetSandboxModeWithSource(sl, sandbox.ModeWorkspaceWrite, sandbox.ModeSourceDelegation)
	if err != nil {
		t.Fatalf("SetSandboxModeWithSource failed: %v", err)
	}
	data := sl.Events()[0].Data.(session.SandboxModeData)
	if data.Source != string(sandbox.ModeSourceDelegation) {
		t.Fatalf("source should be delegation, got %q", data.Source)
	}
}

func TestSandboxModeSessionInterface(t *testing.T) {
	sl := session.NewSessionLog(brand.NewSessionID("sbox-iface"))
	sl.Append(session.SandboxModeData{Mode: string(sandbox.ModeReadOnly)})

	mode, ok := sl.SandboxMode()
	if !ok {
		t.Fatal("SessionLog.SandboxMode() should return from event fold")
	}
	if string(mode) != string(sandbox.ModeReadOnly) {
		t.Fatalf("mode should be read-only, got %s", mode)
	}
}
