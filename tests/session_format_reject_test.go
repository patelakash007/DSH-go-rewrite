package tests

import (
	"encoding/json"
	"testing"

	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/session"
)

func TestSessionFormatRejectUnknownEvent(t *testing.T) {
	header := session.NewSessionHeader(brand.NewSessionID("s1"), "/ws")
	headerBytes, err := header.Marshal()
	if err != nil {
		t.Fatalf("header marshal failed: %v", err)
	}

	known, _ := json.Marshal(session.SessionEvent{
		Seq: 1, Type: session.EventUserMessage, Data: session.UserMessageData{Content: "hi"},
	})
	_, events, err := session.LoadSession(headerBytes, known)
	if err != nil {
		t.Fatalf("known events should pass: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("should load 1 event: %d", len(events))
	}

	unknown := []byte(`{"seq":2,"time":"2026-08-31T00:00:00Z","type":"bogus/event","data":{}}`)
	_, _, err = session.LoadSession(headerBytes, unknown)
	if err == nil {
		t.Fatal("unknown event type should be rejected")
	}
	if !session.IsCorruptEventError(err) {
		t.Fatalf("should be CorruptEventError, got %T", err)
	}
}

func TestSessionFormatCrossVersionReject(t *testing.T) {

	headerBytes := []byte(`{"version":999,"id":"s_x","createdAt":"2026-08-31T00:00:00Z"}`)

	known, _ := json.Marshal(session.SessionEvent{
		Seq: 1, Type: session.EventUserMessage, Data: session.UserMessageData{Content: "hi"},
	})
	_, _, err := session.LoadSession(headerBytes, known)
	if err == nil {
		t.Fatal("session files of different versions should reject each other")
	}
	var unsupported *session.SessionFormatUnsupportedError
	if !asUnsupported(err, &unsupported) {
		t.Fatalf("should be SessionFormatUnsupportedError, got %T", err)
	}
}

func TestSessionKnownEventTypesWhiteList(t *testing.T) {
	if len(session.KNOWN_SESSION_EVENT_TYPES) < 45 {
		t.Fatalf("allowlisted event types count = %d, want >= 45", len(session.KNOWN_SESSION_EVENT_TYPES))
	}

	header := session.NewSessionHeader(brand.NewSessionID("s2"), "/ws")
	headerBytes, _ := header.Marshal()

	for _, et := range session.AllEventTypes {
		data := sampleDataFor(et)
		if data == nil {
			t.Fatalf("missing sample data for %q", et)
		}
		line, err := json.Marshal(session.SessionEvent{Seq: 1, Time: fixedTestTime(), Type: et, Data: data})
		if err != nil {
			t.Fatalf("%q marshal failed: %v", et, err)
		}
		if _, _, err := session.LoadSession(headerBytes, line); err != nil {
			t.Fatalf("%q should pass allowlist validation: %v", et, err)
		}
	}
}
