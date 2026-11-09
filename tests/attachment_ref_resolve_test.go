//

package tests

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JopenChen/dsh-go/pkg/attachment"
	"github.com/JopenChen/dsh-go/pkg/brand"
)

func pngBytes() []byte {
	return []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D}
}

func TestAttachmentSaveAndResolve(t *testing.T) {
	store := attachment.NewStore("")
	data := pngBytes()
	ref, err := store.SaveImage(attachment.SaveImageAttachment{Data: data, MediaType: attachment.MediaPNG, Name: "a.png"})
	if err != nil {
		t.Fatalf("SaveImage failed: %v", err)
	}
	if ref.AttachmentID == (attachment.AttachmentID{}) {
		t.Fatal("should return content-addressed AttachmentID")
	}
	if !strings.HasPrefix(ref.AttachmentID.Raw(), "sha256:") {
		t.Fatalf("AttachmentID should be sha256:<digest>, got %q", ref.AttachmentID.Raw())
	}
	if ref.Bytes != len(data) {
		t.Fatalf("Bytes should record length %d, got %d", len(data), ref.Bytes)
	}
	stored, err := attachment.ResolveReference(store, ref)
	if err != nil {
		t.Fatalf("ResolveReference failed: %v", err)
	}
	if !bytes.Equal(stored.Data, data) {
		t.Fatal("parsed bytes mismatch saved bytes")
	}
}

func TestAttachmentDurableAcrossCompactionAndSession(t *testing.T) {

	dir := t.TempDir()
	store := attachment.NewStore(dir)
	data := pngBytes()

	refA, err := store.SaveImage(attachment.SaveImageAttachment{Data: data, MediaType: attachment.MediaPNG})
	if err != nil {
		t.Fatal(err)
	}

	refB := attachment.ImageAttachmentRef{AttachmentID: refA.AttachmentID, MediaType: refA.MediaType, Bytes: refA.Bytes}
	stored, err := attachment.ResolveReference(store, refB)
	if err != nil {
		t.Fatalf("cross-session durable reference resolution failed: %v", err)
	}
	if !bytes.Equal(stored.Data, data) {
		t.Fatal("cross-session parsed bytes mismatch")
	}

	refOnly := refA
	fileBacked, err := attachment.ResolveFileBacked(dir, refOnly)
	if err != nil {
		t.Fatalf("host file durable path resolution failed: %v", err)
	}
	if !bytes.Equal(fileBacked.Data, data) {
		t.Fatal("host file parsed bytes mismatch")
	}
}

func TestAttachmentMediaTypeAdmission(t *testing.T) {
	store := attachment.NewStore("")

	if _, err := store.SaveImage(attachment.SaveImageAttachment{Data: []byte("not an image"), MediaType: attachment.MediaPNG}); err == nil {
		t.Fatal("bytes mismatching declared media type should be rejected")
	}

	if _, err := store.SaveImage(attachment.SaveImageAttachment{Data: pngBytes(), MediaType: attachment.MediaPNG}); err != nil {
		t.Fatalf("valid PNG should be accepted: %v", err)
	}
}

func TestAttachmentReadUnknownID(t *testing.T) {
	store := attachment.NewStore("")
	_, err := store.ReadImage(attachment.ImageAttachmentRef{
		AttachmentID: brand.NewAttachmentID("sha256:deadbeef"),
		MediaType:    attachment.MediaPNG,
	})
	if err == nil {
		t.Fatal("unknown object should return error")
	}
}

func TestAttachmentHostFileDigestCatch(t *testing.T) {
	dir := t.TempDir()
	store := attachment.NewStore(dir)
	data := pngBytes()
	ref, err := store.SaveImage(attachment.SaveImageAttachment{Data: data, MediaType: attachment.MediaPNG})
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, ref.AttachmentID.Raw()), []byte("tampered"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := attachment.ResolveFileBacked(dir, ref); err == nil {
		t.Fatal("tampered host file should be rejected due to digest mismatch")
	}
}
