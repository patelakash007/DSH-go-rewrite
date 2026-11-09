//

//

//

package spill

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/JopenChen/dsh-go/pkg/brand"
)

// ============================================================================

// ============================================================================

type SpillLocator string

func (l SpillLocator) String() string { return string(l) }

type SpillOwner struct {
	SessionID brand.SessionID `json:"sessionId"`
}

type SpillSource struct {
	ToolName string `json:"toolName"`

	CallID brand.ToolCallID `json:"callId"`

	Label string `json:"label"`
}

type SaveTextSpill struct {
	Owner SpillOwner `json:"owner"`

	Source SpillSource `json:"source"`

	SuggestedName string `json:"suggestedName"`

	Content string `json:"content"`
}

type SpillRef struct {
	Locator SpillLocator `json:"locator"`

	Bytes int `json:"bytes"`

	RetrievalHint string `json:"retrievalHint"`
}

type SpillStore interface {
	SaveText(input SaveTextSpill) (SpillRef, error)
}

// ============================================================================

// ============================================================================

type FileSpillStore struct {
	Root string
}

func NewFileStore(root string) *FileSpillStore {
	return &FileSpillStore{Root: root}
}

func (s *FileSpillStore) SaveText(input SaveTextSpill) (SpillRef, error) {
	if s.Root == "" {
		return SpillRef{}, fmt.Errorf("spill: root not configured")
	}

	sessHash := sha256sum(input.Owner.SessionID.Raw())
	dir := filepath.Join(s.Root, "session-"+sessHash[:16])
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return SpillRef{}, fmt.Errorf("spill: create session dir: %w", err)
	}

	safe := sanitizeName(input.SuggestedName)

	name := randomSuffix() + "-" + safe
	path := filepath.Join(dir, name)

	if err := os.WriteFile(path, []byte(input.Content), 0o600); err != nil {
		return SpillRef{}, fmt.Errorf("spill: write artifact: %w", err)
	}
	return SpillRef{
		Locator:       SpillLocator(path),
		Bytes:         len([]byte(input.Content)),
		RetrievalHint: fmt.Sprintf("read or grep this artifact with the tool at path %q", path),
	}, nil
}

func (s *FileSpillStore) ReadText(locator SpillLocator) (string, error) {
	data, err := os.ReadFile(string(locator))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func sanitizeName(name string) string {
	var sb strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			sb.WriteRune(r)
		} else {
			sb.WriteByte('_')
		}
	}
	out := sb.String()
	if out == "" {
		out = "artifact"
	}

	out = strings.Trim(out, ".")
	if out == "" {
		out = "artifact"
	}
	return out
}

func randomSuffix() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

func sha256sum(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// ============================================================================
// Spill Policy（post-tool / Bash listener）
// ============================================================================

//

func Apply(store SpillStore, input SaveTextSpill, text string, maxInlineBytes, previewBytes int) (inline string, ref *SpillRef, err error) {
	if len([]byte(text)) <= maxInlineBytes {
		return text, nil, nil
	}
	r, serr := store.SaveText(SaveTextSpill{
		Owner:         input.Owner,
		Source:        input.Source,
		SuggestedName: input.SuggestedName,
		Content:       text,
	})
	if serr != nil {

		return text, nil, serr
	}
	if previewBytes <= 0 {
		previewBytes = 1024
	}
	p := preview(text, previewBytes)
	return p, &r, nil
}

func preview(text string, head int) string {
	b := []byte(text)
	tail := head / 4
	if len(b) <= head+tail {
		return text
	}
	tailText := string(b[len(b)-tail:])
	headText := string(b[:head])
	return fmt.Sprintf("%s\n...\n[truncated %d bytes; see spill ref for the full %d bytes]\n%s",
		headText, len(b)-head-tail, len(b), tailText)
}
