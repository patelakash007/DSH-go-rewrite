//

//

//

package brand

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// ============================================================================

// ============================================================================
//

type sessionIDTag struct{}
type toolCallIDTag struct{}
type approvalRequestIDTag struct{}
type jobIDTag struct{}
type skillIDTag struct{}
type attachmentIDTag struct{}
type credentialRefTag struct{}
type workspaceIDTag struct{}
type projectionIDTag struct{}

// ============================================================================

// ============================================================================

//

//

type Branded[T comparable] struct {
	value string
}

func newBranded[T comparable](raw string) Branded[T] {
	return Branded[T]{value: raw}
}

func parseBranded[T comparable](raw string) (Branded[T], error) {
	if raw == "" {
		return Branded[T]{}, fmt.Errorf("brand: empty id for %T", *new(T))
	}
	return Branded[T]{value: raw}, nil
}

func (b Branded[T]) IsZero() bool {
	return b.value == ""
}

func (b Branded[T]) String() string {
	return b.value
}

func (b Branded[T]) Raw() string {
	return b.value
}

func (b Branded[T]) Equal(other Branded[T]) bool {
	return b.value == other.value
}

func (b Branded[T]) MarshalJSON() ([]byte, error) {
	return json.Marshal(b.value)
}

func (b *Branded[T]) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("brand: invalid json for %T: %w", *new(T), err)
	}
	b.value = s
	return nil
}

func (b Branded[T]) Value() (driver.Value, error) {
	return b.value, nil
}

func (b *Branded[T]) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		b.value = ""
	case string:
		b.value = v
	case []byte:
		b.value = string(v)
	default:
		return fmt.Errorf("brand: cannot scan %T into %T", src, b)
	}
	return nil
}

// ============================================================================

// ============================================================================

type SessionID = Branded[sessionIDTag]

type ToolCallID = Branded[toolCallIDTag]

type ApprovalRequestID = Branded[approvalRequestIDTag]

type JobID = Branded[jobIDTag]

type SkillID = Branded[skillIDTag]

type AttachmentID = Branded[attachmentIDTag]

type CredentialRef = Branded[credentialRefTag]

type WorkspaceID = Branded[workspaceIDTag]

type ProjectionID = Branded[projectionIDTag]

// ============================================================================

// ============================================================================

func NewSessionID(raw string) SessionID { return newBranded[sessionIDTag](raw) }

func ParseSessionID(raw string) (SessionID, error) { return parseBranded[sessionIDTag](raw) }

func NewToolCallID(raw string) ToolCallID { return newBranded[toolCallIDTag](raw) }

func ParseToolCallID(raw string) (ToolCallID, error) { return parseBranded[toolCallIDTag](raw) }

func NewApprovalRequestID(raw string) ApprovalRequestID { return newBranded[approvalRequestIDTag](raw) }

func ParseApprovalRequestID(raw string) (ApprovalRequestID, error) {
	return parseBranded[approvalRequestIDTag](raw)
}

func NewJobID(raw string) JobID { return newBranded[jobIDTag](raw) }

func ParseJobID(raw string) (JobID, error) { return parseBranded[jobIDTag](raw) }

func NewSkillID(raw string) SkillID { return newBranded[skillIDTag](raw) }

func ParseSkillID(raw string) (SkillID, error) { return parseBranded[skillIDTag](raw) }

func NewAttachmentID(raw string) AttachmentID { return newBranded[attachmentIDTag](raw) }

func ParseAttachmentID(raw string) (AttachmentID, error) { return parseBranded[attachmentIDTag](raw) }

func NewCredentialRef(raw string) CredentialRef { return newBranded[credentialRefTag](raw) }

func ParseCredentialRef(raw string) (CredentialRef, error) {
	return parseBranded[credentialRefTag](raw)
}

func NewWorkspaceID(raw string) WorkspaceID { return newBranded[workspaceIDTag](raw) }

func ParseWorkspaceID(raw string) (WorkspaceID, error) { return parseBranded[workspaceIDTag](raw) }

func NewProjectionID(raw string) ProjectionID { return newBranded[projectionIDTag](raw) }

func ParseProjectionID(raw string) (ProjectionID, error) { return parseBranded[projectionIDTag](raw) }

// ============================================================================

// ============================================================================

type Bytes[T comparable] struct {
	value []byte
}

func NewBytes[T comparable](raw []byte) Bytes[T] {
	return Bytes[T]{value: raw}
}

func ZeroBytes[T comparable]() Bytes[T] {
	return Bytes[T]{}
}

func (b Bytes[T]) IsZero() bool {
	return len(b.value) == 0
}

func (b Bytes[T]) Bytes() []byte {
	return b.value
}

func (b Bytes[T]) String() string {
	return string(b.value)
}

func (b Bytes[T]) Len() int {
	return len(b.value)
}

func (b Bytes[T]) MarshalJSON() ([]byte, error) {
	return json.Marshal(b.value)
}

func (b *Bytes[T]) UnmarshalJSON(data []byte) error {
	var raw []byte
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("brand: invalid bytes json for %T: %w", *new(T), err)
	}
	b.value = raw
	return nil
}
