package tests

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/JopenChen/dsh-go/pkg/brand"
)

func TestBrandedRoundTrip(t *testing.T) {

	sid := brand.NewSessionID("sess_123456")
	if sid.IsZero() {
		t.Fatal("NewSessionID should not be zero value after construction")
	}
	if sid.String() != "sess_123456" {
		t.Fatalf("String() = %q, want %q", sid.String(), "sess_123456")
	}

	var zero brand.SessionID
	if !zero.IsZero() {
		t.Fatal("zero value SessionID should have IsZero()==true")
	}

	other := brand.NewSessionID("sess_123456")
	if !sid.Equal(other) {
		t.Fatal("SessionIDs constructed from same string should be equal")
	}
	diff := brand.NewSessionID("sess_999")
	if sid.Equal(diff) {
		t.Fatal("SessionIDs constructed from different strings should not be equal")
	}
}

func TestBrandedParse(t *testing.T) {
	if _, err := brand.ParseToolCallID(""); err == nil {
		t.Fatal("ParseToolCallID on empty string should return error")
	}
	parsed, err := brand.ParseToolCallID("call_abc")
	if err != nil {
		t.Fatalf("ParseToolCallID on valid string failed: %v", err)
	}
	if parsed.String() != "call_abc" {
		t.Fatalf("Parse result = %q, want %q", parsed.String(), "call_abc")
	}
}

func TestBrandedJSON(t *testing.T) {
	original := brand.NewJobID("job_42")

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	if string(data) != `"job_42"` {
		t.Fatalf("Marshal output = %s, want \"job_42\"", string(data))
	}

	var restored brand.JobID
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if !restored.Equal(original) {
		t.Fatalf("mismatch after JSON round-trip: %q vs %q", restored.String(), original.String())
	}

	var bad brand.SkillID
	if err := json.Unmarshal([]byte(`{"a":1}`), &bad); err == nil {
		t.Fatal("non-string JSON should fail to unmarshal")
	}
}

// (SessionID -> brand.Branded[brand.sessionIDTag], ToolCallID -> brand.Branded[brand.toolCallIDTag])

func TestBrandedTypeIsolation(t *testing.T) {
	sid := brand.NewSessionID("s1")
	tcid := brand.NewToolCallID("t1")
	job := brand.NewJobID("j1")

	if typeName(sid) == typeName(tcid) {
		t.Fatal("underlying type names of SessionID and ToolCallID should differ")
	}
	if typeName(job) == typeName(tcid) {
		t.Fatal("underlying type names of JobID and ToolCallID should differ")
	}

	t.Logf("SessionID type = %s", typeName(sid))
	t.Logf("ToolCallID type = %s", typeName(tcid))
	t.Logf("JobID type = %s", typeName(job))
}

func typeName(x any) string {
	return fmt.Sprintf("%T", x)
}

func TestBrandedSQL(t *testing.T) {
	sid := brand.NewSessionID("sess_sql")

	v, err := sid.Value()
	if err != nil {
		t.Fatalf("Value() failed: %v", err)
	}
	if v != "sess_sql" {
		t.Fatalf("Value() = %v, want sess_sql", v)
	}

	var restored brand.SessionID
	if err := restored.Scan(v); err != nil {
		t.Fatalf("Scan(string) failed: %v", err)
	}
	if !restored.Equal(sid) {
		t.Fatal("should equal original value after Scan")
	}

	var fromBytes brand.SessionID
	if err := fromBytes.Scan([]byte("sess_sql")); err != nil {
		t.Fatalf("Scan([]byte) failed: %v", err)
	}
	if !fromBytes.Equal(sid) {
		t.Fatal("should equal original value after Scan([]byte)")
	}

	var fromNil brand.SessionID
	if err := fromNil.Scan(nil); err != nil {
		t.Fatalf("Scan(nil) failed: %v", err)
	}
	if !fromNil.IsZero() {
		t.Fatal("should be zero value after Scan(nil)")
	}

	if err := fromNil.Scan(42); err == nil {
		t.Fatal("Scan(int) should return error")
	}
}

func TestBytesBranded(t *testing.T) {
	raw := []byte("hello world \x00 binary")
	bs := brand.NewBytes[brand.AttachmentID](raw)

	if bs.IsZero() {
		t.Fatal("NewBytes should not be zero value after construction")
	}
	if bs.Len() != len(raw) {
		t.Fatalf("Len() = %d, want %d", bs.Len(), len(raw))
	}
	if string(bs.Bytes()) != string(raw) {
		t.Fatal("Bytes() should restore original bytes")
	}

	data, err := json.Marshal(bs)
	if err != nil {
		t.Fatalf("Bytes Marshal failed: %v", err)
	}
	var restored brand.Bytes[brand.AttachmentID]
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Bytes Unmarshal failed: %v", err)
	}
	if string(restored.Bytes()) != string(raw) {
		t.Fatal("Bytes JSON round-trip should restore original bytes")
	}

	if !brand.ZeroBytes[brand.AttachmentID]().IsZero() {
		t.Fatal("ZeroBytes should have IsZero()==true")
	}
}
