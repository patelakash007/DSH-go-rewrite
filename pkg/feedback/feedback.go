//

//

// - list/put/delete。
package feedback

import (
	"context"
	"fmt"
	"time"

	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/storage"
)

// ============================================================================

// ============================================================================

type Rating int

const (
	RatingThumbsDown Rating = -1
	RatingNone       Rating = 0
	RatingThumbsUp   Rating = 1
)

type Feedback struct {
	SessionID brand.SessionID `json:"sessionId"`

	MessageID string `json:"messageId"`

	Rating Rating `json:"rating"`

	Note string `json:"note,omitempty"`

	Version uint64 `json:"version"`

	CreatedAt time.Time `json:"createdAt"`

	UpdatedAt time.Time `json:"updatedAt"`
}

// ============================================================================

// ============================================================================

type FailCode string

const (
	CodeVersionConflict FailCode = "VERSION_CONFLICT"
	CodeSessionNotFound FailCode = "SESSION_NOT_FOUND"
	CodeNotFound        FailCode = "NOT_FOUND"
)

type FailError struct {
	Code FailCode
	Msg  string
}

func (e *FailError) Error() string { return string(e.Code) + ": " + e.Msg }

func fail(code FailCode, format string, args ...any) error {
	return &FailError{Code: code, Msg: fmt.Sprintf(format, args...)}
}

func CodeOf(err error) FailCode {
	if err == nil {
		return ""
	}
	if fe, ok := err.(*FailError); ok {
		return fe.Code
	}
	return ""
}

// ============================================================================
// Store
// ============================================================================

type Store struct {
	domain *storage.Domain[Feedback]
}

func New(backend storage.Backend) *Store {
	return &Store{domain: storage.NewDomain[Feedback](backend, "feedback")}
}

func keyOf(session brand.SessionID, messageID string) string {
	if session.IsZero() {
		return "_unowned_" + messageID
	}
	return session.Raw() + "::" + messageID
}

func (s *Store) Put(ctx context.Context, session brand.SessionID, messageID string, rating Rating, note string, expectedVersion *uint64) (uint64, error) {
	if err := ValidateRating(rating); err != nil {
		return 0, err
	}
	if err := ValidateNote(note); err != nil {
		return 0, err
	}
	if session.IsZero() {
		return 0, fail(CodeSessionNotFound, "missing session id")
	}
	key := keyOf(session, messageID)
	now := time.Now()

	existing, ver, err := s.domain.Get(ctx, key)
	var expected uint64
	created := false
	if err != nil {
		if !isNotFound(err) {
			return 0, err
		}

		created = true
		expected = 0
	} else {
		expected = ver
		_ = existing
	}

	if expectedVersion != nil {
		if created {
			if *expectedVersion != 0 {
				return 0, fail(CodeVersionConflict, "feedback for message %q does not exist (expected v%d)", messageID, *expectedVersion)
			}
		} else if *expectedVersion != expected {
			return 0, fail(CodeVersionConflict, "feedback for message %q version mismatch (expected %d, now %d)", messageID, *expectedVersion, expected)
		}
	}

	fb := Feedback{
		SessionID: session,
		MessageID: messageID,
		Rating:    rating,
		Note:      note,
		Version:   expected + 1,
		CreatedAt: existing.CreatedAt,
		UpdatedAt: now,
	}
	if created {
		fb.CreatedAt = now
	}
	newVer, err := s.domain.Put(ctx, key, fb, expected)
	if err != nil {

		return 0, fail(CodeVersionConflict, "cas put failed: %v", err)
	}
	return newVer, nil
}

func (s *Store) Get(ctx context.Context, session brand.SessionID, messageID string) (*Feedback, error) {
	fb, _, err := s.domain.Get(ctx, keyOf(session, messageID))
	if err != nil {
		if isNotFound(err) {
			return nil, fail(CodeNotFound, "feedback for message %q not found", messageID)
		}
		return nil, err
	}
	return &fb, nil
}

func (s *Store) Delete(ctx context.Context, session brand.SessionID, messageID string) error {
	return s.domain.Delete(ctx, keyOf(session, messageID))
}

func (s *Store) List(ctx context.Context, session brand.SessionID) ([]Feedback, error) {
	keys, err := s.domain.List(ctx)
	if err != nil {
		return nil, err
	}
	prefix := session.Raw() + "::"
	var out []Feedback
	for _, k := range keys {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			fb, _, err := s.domain.Get(ctx, k)
			if err == nil {
				out = append(out, fb)
			}
		}
	}
	return out, nil
}

func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*storage.ErrKeyNotFound)
	return ok
}
