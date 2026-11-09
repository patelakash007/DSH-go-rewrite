package tests

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/JopenChen/dsh-go/pkg/storage"
)

func makeBackends(t *testing.T) (fileKV, sqliteKV, memKV storage.Backend, cleanup func()) {
	t.Helper()
	dir := t.TempDir()

	fk, err := storage.NewFileKV(filepath.Join(dir, "filekv"))
	if err != nil {
		t.Fatalf("NewFileKV failed: %v", err)
	}
	sk, err := storage.NewSQLiteKV(filepath.Join(dir, "sqlite.db"))
	if err != nil {
		t.Fatalf("NewSQLiteKV failed: %v", err)
	}
	return fk, sk, storage.NewMemoryKV(), func() {
		_ = sk.Close()
	}
}

func TestStorageCASMismatchConsistent(t *testing.T) {
	fileKV, sqliteKV, memKV, cleanup := makeBackends(t)
	defer cleanup()
	ctx := context.Background()

	for name, backend := range map[string]storage.Backend{
		"filekv":   fileKV,
		"sqlitekv": sqliteKV,
		"memorykv": memKV,
	} {
		t.Run(name, func(t *testing.T) {

			v1, err := backend.Put(ctx, "k1", []byte("a"), 0)
			if err != nil {
				t.Fatalf("create failed: %v", err)
			}
			if v1 != 1 {
				t.Fatalf("initial version should be 1: %d", v1)
			}

			_, err = backend.Put(ctx, "k1", []byte("b"), 99)
			if !storage.IsCASMismatch(err) {
				t.Fatalf("should return ErrCASMismatch, got %v", err)
			}
			var mismatch *storage.ErrCASMismatch
			if !asCASMismatch(err, &mismatch) {
				t.Fatalf("error type should be *ErrCASMismatch: %T", err)
			}
			if mismatch.ExpectedVersion != 99 || mismatch.ActualVersion != 1 {
				t.Fatalf("CAS fields unexpected: %+v", mismatch)
			}

			v2, err := backend.Put(ctx, "k1", []byte("b"), 1)
			if err != nil {
				t.Fatalf("correct version update failed: %v", err)
			}
			if v2 != 2 {
				t.Fatalf("second version should be 2: %d", v2)
			}

			if _, err := backend.Put(ctx, "k1", []byte("c"), 0); !storage.IsCASMismatch(err) {
				t.Fatalf("creating existing key with 0 should conflict: %v", err)
			}

			data, version, err := backend.Get(ctx, "k1")
			if err != nil {
				t.Fatalf("Get failed: %v", err)
			}
			if string(data) != "b" || version != 2 {
				t.Fatalf("read result unexpected: %q v%d", data, version)
			}
		})
	}
}

func asCASMismatch(err error, target **storage.ErrCASMismatch) bool {
	e, ok := err.(*storage.ErrCASMismatch)
	if ok {
		*target = e
	}
	return ok
}

func TestStorageTypedDomain(t *testing.T) {
	_, sqliteKV, _, cleanup := makeBackends(t)
	defer cleanup()
	ctx := context.Background()

	type feedback struct {
		Rating int    `json:"rating"`
		Note   string `json:"note"`
	}

	domain := storage.NewDomain[feedback](sqliteKV, "feedback")

	if _, err := domain.Put(ctx, "msg_1", feedback{Rating: 5, Note: "good"}, 0); err != nil {
		t.Fatalf("create failed: %v", err)
	}

	fb, version, err := domain.Get(ctx, "msg_1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if fb.Rating != 5 || fb.Note != "good" {
		t.Fatalf("read typed value unexpected: %+v", fb)
	}

	if _, err := domain.Put(ctx, "msg_1", feedback{Rating: 4}, version); err != nil {
		t.Fatalf("CAS update failed: %v", err)
	}

	if _, err := domain.Put(ctx, "msg_1", feedback{Rating: 1}, version); !storage.IsCASMismatch(err) {
		t.Fatalf("updating stale version should conflict: %v", err)
	}

	if _, err := domain.Put(ctx, "msg_2", feedback{Rating: 3}, 0); err != nil {
		t.Fatalf("msg_2 create failed: %v", err)
	}
	keys, err := domain.List(ctx)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("domain key count = %d, want 2: %v", len(keys), keys)
	}
}

func TestStorageMigration(t *testing.T) {
	fileKV, sqliteKV, _, cleanup := makeBackends(t)
	defer cleanup()
	ctx := context.Background()

	_, _ = fileKV.Put(ctx, "a", []byte("1"), 0)
	_, _ = fileKV.Put(ctx, "b/x", []byte("2"), 0)
	_, _ = fileKV.Put(ctx, "c", []byte("3"), 0)

	migrated, skipped, err := storage.Migrate(ctx, fileKV, sqliteKV)
	if err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}
	if migrated != 3 {
		t.Fatalf("migrated count = %d, want 3", migrated)
	}
	if len(skipped) != 0 {
		t.Fatalf("first migration should not skip: %v", skipped)
	}

	for _, key := range []string{"a", "b/x", "c"} {
		data, _, err := sqliteKV.Get(ctx, key)
		if err != nil {
			t.Fatalf("target read %q failed: %v", key, err)
		}
		if key == "a" && string(data) != "1" {
			t.Fatalf("data mismatch after migration: %q", data)
		}
	}

	migrated2, skipped2, err := storage.Migrate(ctx, fileKV, sqliteKV)
	if err != nil {
		t.Fatalf("second migration failed: %v", err)
	}
	if migrated2 != 0 || len(skipped2) != 3 {
		t.Fatalf("second migration should skip all: migrated=%d skipped=%v", migrated2, skipped2)
	}
}

func TestStorageKeyNotFound(t *testing.T) {
	_, _, memKV, cleanup := makeBackends(t)
	defer cleanup()
	ctx := context.Background()

	if _, _, err := memKV.Get(ctx, "missing"); !storage.IsKeyNotFound(err) {
		t.Fatalf("should return ErrKeyNotFound, got %v", err)
	}
}
