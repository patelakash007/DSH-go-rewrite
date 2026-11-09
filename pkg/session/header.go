//

//

package session

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/JopenChen/dsh-go/pkg/brand"
)

const SessionFormatVersion = 1

type HeaderOrigin string

const (
	OriginCreated HeaderOrigin = "created"
	OriginFork    HeaderOrigin = "fork"
	OriginResume  HeaderOrigin = "resume"
)

type SessionHeader struct {
	Version int `json:"version"`

	ID brand.SessionID `json:"id"`

	CreatedAt time.Time `json:"createdAt"`

	Cwd string `json:"cwd,omitempty"`

	ParentSession brand.SessionID `json:"parentSession,omitempty"`

	SeedLength uint64 `json:"seedLength,omitempty"`

	Origin HeaderOrigin `json:"origin,omitempty"`

	DelegationDepth int `json:"delegationDepth,omitempty"`

	AgentPreset string `json:"agentPreset,omitempty"`
}

func NewSessionHeader(id brand.SessionID, cwd string) *SessionHeader {
	return &SessionHeader{
		Version:         SessionFormatVersion,
		ID:              id,
		CreatedAt:       time.Now().UTC(),
		Cwd:             cwd,
		Origin:          OriginCreated,
		DelegationDepth: 0,
	}
}

func (h *SessionHeader) Marshal() ([]byte, error) {
	return json.Marshal(h)
}

func UnmarshalSessionHeader(data []byte) (*SessionHeader, error) {
	var h SessionHeader
	if err := json.Unmarshal(data, &h); err != nil {
		return nil, fmt.Errorf("session header: unmarshal: %w", err)
	}
	if h.Version != SessionFormatVersion {
		return nil, &SessionFormatUnsupportedError{
			Version: h.Version,
		}
	}
	return &h, nil
}

func (h *SessionHeader) Validate() error {
	if h.Version != SessionFormatVersion {
		return &SessionFormatUnsupportedError{Version: h.Version}
	}
	if h.ID.IsZero() {
		return fmt.Errorf("session header: missing session id")
	}
	if h.CreatedAt.IsZero() {
		return fmt.Errorf("session header: missing created-at")
	}
	return nil
}

func (h *SessionHeader) Fork(childID brand.SessionID) *SessionHeader {
	return &SessionHeader{
		Version:         SessionFormatVersion,
		ID:              childID,
		CreatedAt:       time.Now().UTC(),
		Cwd:             h.Cwd,
		ParentSession:   h.ID,
		SeedLength:      0,
		Origin:          OriginFork,
		DelegationDepth: h.DelegationDepth + 1,
		AgentPreset:     h.AgentPreset,
	}
}

type SessionFormatUnsupportedError struct {
	Version int
}

func (e *SessionFormatUnsupportedError) Error() string {
	return fmt.Sprintf("session header: unsupported format version %d (want %d)", e.Version, SessionFormatVersion)
}

func AsHeaderOrigin(s string) HeaderOrigin {
	return HeaderOrigin(s)
}
