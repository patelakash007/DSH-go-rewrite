//

//

//

package session

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/JopenChen/dsh-go/pkg/brand"
)

// ============================================================================

// ============================================================================

type SessionReferenceErrorCode string

const (
	RefCodeInvalidConfig SessionReferenceErrorCode = "SESSION_REFERENCE_INVALID_CONFIG"

	RefCodeInvalidReference SessionReferenceErrorCode = "SESSION_REFERENCE_INVALID_REFERENCE"

	RefCodeSelfReference SessionReferenceErrorCode = "SESSION_REFERENCE_SELF_REFERENCE"

	RefCodeTooMany SessionReferenceErrorCode = "SESSION_REFERENCE_TOO_MANY"

	RefCodeReadFailed SessionReferenceErrorCode = "SESSION_REFERENCE_READ_FAILED"

	RefCodeBudgetExceeded SessionReferenceErrorCode = "SESSION_REFERENCE_BUDGET_EXCEEDED"

	RefCodeCancelled SessionReferenceErrorCode = "SESSION_REFERENCE_CANCELLED"

	RefCodeSessionNotFound SessionReferenceErrorCode = "SESSION_NOT_FOUND"

	RefCodeFileNotFound SessionReferenceErrorCode = "FILE_NOT_FOUND"

	RefCodeFileOutOfWorkspace SessionReferenceErrorCode = "FILE_OUT_OF_WORKSPACE"
)

type SessionReferenceError struct {
	Code SessionReferenceErrorCode
	Msg  string
}

func (e *SessionReferenceError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Msg)
}

func newRefError(code SessionReferenceErrorCode, format string, args ...any) error {
	return &SessionReferenceError{Code: code, Msg: fmt.Sprintf(format, args...)}
}

func RefCodeFrom(err error) SessionReferenceErrorCode {
	if err == nil {
		return ""
	}
	if re, ok := err.(*SessionReferenceError); ok {
		return re.Code
	}
	return ""
}

// ============================================================================

// ============================================================================

type MentionKind string

const (
	MentionSession MentionKind = "session"

	MentionFile MentionKind = "file"
)

type Mention struct {
	Kind MentionKind

	Raw string

	Value string

	Start int
	End   int
}

var sessionMentionRe = regexp.MustCompile(`@session/([A-Za-z0-9_.\-]+)`)

var fileMentionRe = regexp.MustCompile(`#([^\s#]+)`)

const maxReferencesPerMessage = 16

func ParseMentions(text string) []Mention {
	var out []Mention

	for _, loc := range sessionMentionRe.FindAllStringSubmatchIndex(text, -1) {
		raw := text[loc[0]:loc[1]]
		val := text[loc[2]:loc[3]]
		out = append(out, Mention{
			Kind:  MentionSession,
			Raw:   raw,
			Value: val,
			Start: loc[0],
			End:   loc[1],
		})
	}

	for _, loc := range fileMentionRe.FindAllStringSubmatchIndex(text, -1) {

		raw := text[loc[0]:loc[1]]
		val := text[loc[2]:loc[3]]
		out = append(out, Mention{
			Kind:  MentionFile,
			Raw:   raw,
			Value: val,
			Start: loc[0],
			End:   loc[1],
		})
	}

	sortMentionsByPos(out)
	return out
}

func sortMentionsByPos(m []Mention) {
	for i := 1; i < len(m); i++ {
		for j := i; j > 0 && m[j-1].Start > m[j].Start; j-- {
			m[j-1], m[j] = m[j], m[j-1]
		}
	}
}

func stripMentions(text string, mentions []Mention) string {
	if len(mentions) == 0 {
		return text
	}
	var sb strings.Builder
	last := 0
	for _, m := range mentions {
		sb.WriteString(text[last:m.Start])
		last = m.End
	}
	sb.WriteString(text[last:])
	return sb.String()
}

// ============================================================================
// PreparedReferencedMessage
// ============================================================================

type SessionReferenceInput struct {
	SessionID brand.SessionID

	Label string
}

type PreparedReferencedMessage struct {
	Content string

	AdditionalContext string

	References []ReferenceResolution
}

type ReferenceResolution struct {
	Kind  MentionKind `json:"kind"`
	Raw   string      `json:"raw,omitempty"`
	Value string      `json:"value"`
	Label string      `json:"label,omitempty"`
	Slice string      `json:"slice,omitempty"`
}

// ============================================================================

// ============================================================================

//

type SessionReferenceResolver interface {
	ResolveSession(id brand.SessionID) (string, error)

	ResolveFile(path string) (string, error)

	WorkspaceRoot() string
}

//

type WorkspaceFileResolver struct {
	Root string
}

func (r *WorkspaceFileResolver) WorkspaceRoot() string { return r.Root }

func (r *WorkspaceFileResolver) ResolveSession(brand.SessionID) (string, error) {
	return "", newRefError(RefCodeInvalidConfig, "workspace file resolver has no session capability")
}

func (r *WorkspaceFileResolver) ResolveFile(path string) (string, error) {
	root, err := filepath.Abs(r.Root)
	if err != nil {
		return "", newRefError(RefCodeInvalidConfig, "bad workspace root %q: %v", r.Root, err)
	}

	abs := path
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(root, abs)
	}
	abs, err = filepath.Abs(abs)
	if err != nil {
		return "", newRefError(RefCodeInvalidReference, "bad file path %q: %v", path, err)
	}

	if abs == root || !within(abs, root) {
		return "", newRefError(RefCodeFileOutOfWorkspace, "file %q outside workspace root %q", path, root)
	}

	info, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return "", newRefError(RefCodeFileNotFound, "file %q not found", path)
		}
		return "", newRefError(RefCodeReadFailed, "stat %q: %v", path, err)
	}
	kind := "file"
	if info.IsDir() {
		kind = "directory"
	}

	return fmt.Sprintf("file %q (%s, %d bytes)", path, kind, info.Size()), nil
}

func within(abs, root string) bool {
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// ============================================================================

// ============================================================================

type PrepareRequest struct {
	AgentSessionID brand.SessionID

	Text string

	Resolver SessionReferenceResolver

	MaxContextBytes int
}

//

func Prepare(req PrepareRequest) (*PreparedReferencedMessage, error) {
	if req.Resolver == nil {
		return nil, newRefError(RefCodeInvalidConfig, "missing reference resolver")
	}
	mentions := ParseMentions(req.Text)
	if len(mentions) > maxReferencesPerMessage {
		return nil, newRefError(RefCodeTooMany, "%d references exceed limit %d", len(mentions), maxReferencesPerMessage)
	}

	content := stripMentions(req.Text, mentions)

	if len(mentions) == 0 {
		return &PreparedReferencedMessage{Content: content}, nil
	}

	var sb strings.Builder
	var res []ReferenceResolution
	var total int
	for _, m := range mentions {
		var slice string
		var err error
		switch m.Kind {
		case MentionSession:

			if !req.AgentSessionID.IsZero() && m.Value == req.AgentSessionID.Raw() {
				return nil, newRefError(RefCodeSelfReference, "self reference to %q", m.Value)
			}
			slice, err = req.Resolver.ResolveSession(brand.NewSessionID(m.Value))
		case MentionFile:
			slice, err = req.Resolver.ResolveFile(m.Value)
		default:
			return nil, newRefError(RefCodeInvalidReference, "unknown mention kind %q", m.Kind)
		}
		if err != nil {
			return nil, err
		}
		total += len(slice)
		if req.MaxContextBytes > 0 && total > req.MaxContextBytes {
			return nil, newRefError(RefCodeBudgetExceeded, "aggregated context exceeds %d bytes", req.MaxContextBytes)
		}
		res = append(res, ReferenceResolution{
			Kind:  m.Kind,
			Raw:   m.Raw,
			Value: m.Value,
			Slice: slice,
		})
		sb.WriteString("### referenced: ")
		sb.WriteString(m.Value)
		sb.WriteString("\n")
		sb.WriteString(slice)
		sb.WriteString("\n\n")
	}

	return &PreparedReferencedMessage{
		Content:           content,
		AdditionalContext: strings.TrimRight(sb.String(), "\n"),
		References:        res,
	}, nil
}
