//

package tests

import (
	"strings"
	"testing"

	"github.com/JopenChen/dsh-go/pkg/fs"
)

func TestFsObsPolicyRejectsUnobservedWrite(t *testing.T) {
	root := t.TempDir()
	local := fs.NewLocal(root)
	obs := fs.NewObservationPolicy()
	svc := fs.NewPolicyFS(local, obs)

	target, _ := local.Resolve("a.txt")

	_ = target
	existing, _ := local.Resolve("existing.txt")
	if _, err := svc.Write("owner1", existing, "hello"); err != nil {
		t.Fatalf("first write (createIfAbsent, non-existent file) should succeed: %v", err)
	}

	_, err := svc.Write("owner1", existing, "hello2")
	if fs.Code(err) != fs.FSErrNotObserved {
		t.Fatalf("unobserved version with existing file should be FS_NOT_OBSERVED, got %v", err)
	}
}

func TestFsReadThenWriteGuarded(t *testing.T) {
	root := t.TempDir()
	local := fs.NewLocal(root)
	obs := fs.NewObservationPolicy()
	svc := fs.NewPolicyFS(local, obs)

	target, _ := local.Resolve("g.txt")

	if _, err := svc.Write("o", target, "v1"); err != nil {
		t.Fatal(err)
	}

	svc.ReflectObservation("o", target)

	res, err := svc.Write("o", target, "v2")
	if err != nil {
		t.Fatalf("replaceIfVersion after observation should succeed: %v", err)
	}
	if res.Operation != "update" {
		t.Fatalf("should be update, got %s", res.Operation)
	}
	got, _ := local.ReadText(target)
	if got != "v2" {
		t.Fatalf("content should be v2, got %q", got)
	}
}

func TestFsConcurrentEditStaleVersion(t *testing.T) {
	root := t.TempDir()
	local := fs.NewLocal(root)
	obs := fs.NewObservationPolicy()
	svc := fs.NewPolicyFS(local, obs)

	target, _ := local.Resolve("c.txt")
	if _, err := svc.Write("o", target, "aaa\nbbb\n"); err != nil {
		t.Fatal(err)
	}
	svc.ReflectObservation("o", target)

	if _, err := svc.Edit("o", target, fs.FsEditRequest{OldString: "aaa", NewString: "AAA"}); err != nil {
		t.Fatalf("first edit should succeed: %v", err)
	}

	edit := fs.FsEditRequest{OldString: "bbb", NewString: "BBB"}
	if _, err := svc.Edit("o", target, edit); err == nil {
		t.Fatal("stale version edit should return error")
	} else if fs.Code(err) != fs.FSErrStaleVersion {
		t.Fatalf("should be FS_STALE_VERSION, got %v", err)
	}
}

func TestFsEditNotFoundAndAmbiguous(t *testing.T) {
	local := fs.NewLocal(t.TempDir())
	target, _ := local.Resolve("e.txt")
	if _, err := local.WriteText(target, "x y x\n", nil); err != nil {
		t.Fatal(err)
	}

	if _, err := local.EditText(target, fs.FsEditRequest{OldString: "x", NewString: "z"}, nil); fs.Code(err) != fs.FSErrAmbiguousEdit {
		t.Fatalf("multiple matches should be AmbiguousEdit, got %v", err)
	}

	if _, err := local.EditText(target, fs.FsEditRequest{OldString: "q", NewString: "z"}, nil); fs.Code(err) != fs.FSErrEditNotFound {
		t.Fatalf("no match should be EditNotFound, got %v", err)
	}
	if _, err := local.EditText(target, fs.FsEditRequest{OldString: "x", NewString: "z", ReplaceAll: true}, nil); err != nil {
		t.Fatalf("replaceAll should succeed: %v", err)
	}
}

func TestFsReadBytesTooLarge(t *testing.T) {
	local := fs.NewLocal(t.TempDir())
	target, _ := local.Resolve("big.txt")
	if _, err := local.WriteText(target, strings.Repeat("a", 4096), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := local.ReadBytes(target, 100); fs.Code(err) != fs.FSErrTooLarge {
		t.Fatalf("exceeding limit should be FS_TOO_LARGE, got %v", err)
	}
}

func TestFsVersionStability(t *testing.T) {
	local := fs.NewLocal(t.TempDir())
	target, _ := local.Resolve("v.txt")
	if _, err := local.WriteText(target, "stable", nil); err != nil {
		t.Fatal(err)
	}
	v1, _ := local.ObservedVersion(target)
	v2, _ := local.ObservedVersion(target)
	if v1 != v2 {
		t.Fatalf("versions across two calls in same state should match: %s vs %s", v1, v2)
	}
}
