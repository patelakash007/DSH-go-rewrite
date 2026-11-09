//

//

//

package attachment

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/JopenChen/dsh-go/pkg/brand"
)

// ============================================================================

// ============================================================================

type ImageMediaType string

const (
	MediaPNG  ImageMediaType = "image/png"
	MediaJPEG ImageMediaType = "image/jpeg"
	MediaWebP ImageMediaType = "image/webp"
	MediaGIF  ImageMediaType = "image/gif"
)

type AttachmentID = brand.AttachmentID

type ImageAttachmentRef struct {
	AttachmentID AttachmentID `json:"attachmentId"`

	MediaType ImageMediaType `json:"mediaType"`

	Bytes int `json:"bytes"`

	Width int `json:"width,omitempty"`

	Height int `json:"height,omitempty"`

	Name string `json:"name,omitempty"`
}

type StoredImageAttachment struct {
	Ref  ImageAttachmentRef `json:"ref"`
	Data []byte             `json:"data"`
}

type SaveImageAttachment struct {
	Data []byte

	MediaType ImageMediaType

	Name string

	Width  int
	Height int
}

// ============================================================================

// ============================================================================

type Store interface {
	SaveImage(input SaveImageAttachment) (ImageAttachmentRef, error)

	ReadImage(ref ImageAttachmentRef) (StoredImageAttachment, error)

	ImageHostPath(ref ImageAttachmentRef) (string, bool)
}

// ============================================================================

// ============================================================================

//

type FileBackedStore struct {
	mu   sync.RWMutex
	dir  string
	objs map[string][]byte
}

func NewStore(dir string) *FileBackedStore {
	return &FileBackedStore{
		dir:  dir,
		objs: map[string][]byte{},
	}
}

func contentID(data []byte) AttachmentID {
	sum := sha256.Sum256(data)
	return brand.NewAttachmentID("sha256:" + hex.EncodeToString(sum[:]))
}

var errUnsupportedMedia = errors.New("attachment: unsupported media type")

func validateMediaType(mt ImageMediaType, data []byte) error {

	sig := map[ImageMediaType]func([]byte) bool{
		MediaPNG: func(b []byte) bool {
			return len(b) >= 8 && b[0] == 0x89 && b[1] == 0x50 && b[2] == 0x4E && b[3] == 0x47
		},
		MediaJPEG: func(b []byte) bool { return len(b) >= 3 && b[0] == 0xFF && b[1] == 0xD8 && b[2] == 0xFF },
		MediaWebP: func(b []byte) bool { return len(b) >= 12 && string(b[0:4]) == "RIFF" && string(b[8:12]) == "WEBP" },
		MediaGIF:  func(b []byte) bool { return len(b) >= 6 && string(b[0:4]) == "GIF8" },
	}
	check, ok := sig[mt]
	if !ok {
		return errUnsupportedMedia
	}
	if !check(data) {
		return fmt.Errorf("attachment: bytes do not match declared media type %q", string(mt))
	}
	return nil
}

func (s *FileBackedStore) SaveImage(input SaveImageAttachment) (ImageAttachmentRef, error) {
	if len(input.Data) == 0 {
		return ImageAttachmentRef{}, errors.New("attachment: empty image data")
	}
	if err := validateMediaType(input.MediaType, input.Data); err != nil {
		return ImageAttachmentRef{}, err
	}
	id := contentID(input.Data)
	ref := ImageAttachmentRef{
		AttachmentID: id,
		MediaType:    input.MediaType,
		Bytes:        len(input.Data),
		Width:        input.Width,
		Height:       input.Height,
		Name:         input.Name,
	}
	s.mu.Lock()
	s.objs[id.Raw()] = append([]byte(nil), input.Data...)
	s.mu.Unlock()

	if s.dir != "" {
		_ = writeHostFile(s.dir, ref.AttachmentID, input.Data)
	}
	return ref, nil
}

func (s *FileBackedStore) ReadImage(ref ImageAttachmentRef) (StoredImageAttachment, error) {
	s.mu.RLock()
	data, ok := s.objs[ref.AttachmentID.Raw()]
	s.mu.RUnlock()
	if !ok {
		return StoredImageAttachment{}, fmt.Errorf("attachment: object %q not found", ref.AttachmentID.Raw())
	}

	want := contentID(data)
	if want != ref.AttachmentID {
		return StoredImageAttachment{}, fmt.Errorf("attachment: digest mismatch for %q", ref.AttachmentID.Raw())
	}
	return StoredImageAttachment{Ref: ref, Data: append([]byte(nil), data...)}, nil
}

func (s *FileBackedStore) ImageHostPath(ref ImageAttachmentRef) (string, bool) {
	if s.dir == "" {
		return "", false
	}
	return filepath.Join(s.dir, ref.AttachmentID.Raw()), true
}

func writeHostFile(dir string, id AttachmentID, data []byte) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, id.Raw()), data, 0o644)
}

// ============================================================================

// ============================================================================

//

//

func ResolveReference(store Store, ref ImageAttachmentRef) (StoredImageAttachment, error) {
	if store != nil {
		return store.ReadImage(ref)
	}
	return StoredImageAttachment{}, errors.New("attachment: no store available for durable reference resolution")
}

func ResolveFileBacked(dir string, ref ImageAttachmentRef) (StoredImageAttachment, error) {
	p := filepath.Join(dir, ref.AttachmentID.Raw())
	data, err := os.ReadFile(p)
	if err != nil {
		return StoredImageAttachment{}, fmt.Errorf("attachment: read host file for %q: %w", ref.AttachmentID.Raw(), err)
	}
	if contentID(data) != ref.AttachmentID {
		return StoredImageAttachment{}, fmt.Errorf("attachment: host file digest mismatch for %q", ref.AttachmentID.Raw())
	}
	return StoredImageAttachment{Ref: ref, Data: data}, nil
}
