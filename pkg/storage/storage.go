//

//

package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// ============================================================================

// ============================================================================

type ErrCASMismatch struct {
	Key             string
	ExpectedVersion uint64
	ActualVersion   uint64
}

func (e *ErrCASMismatch) Error() string {
	return fmt.Sprintf("storage: CAS version mismatch for key %q: expected %d, actual %d",
		e.Key, e.ExpectedVersion, e.ActualVersion)
}

// ============================================================================

// ============================================================================

type Backend interface {
	Get(ctx context.Context, key string) (data []byte, version uint64, err error)

	Put(ctx context.Context, key string, data []byte, expectedVersion uint64) (version uint64, err error)

	Delete(ctx context.Context, key string) error

	List(ctx context.Context, prefix string) (keys []string, err error)
}

type ErrKeyNotFound struct {
	Key string
}

func (e *ErrKeyNotFound) Error() string {
	return fmt.Sprintf("storage: key not found: %q", e.Key)
}

func IsKeyNotFound(err error) bool {
	_, ok := err.(*ErrKeyNotFound)
	return ok
}

func IsCASMismatch(err error) bool {
	_, ok := err.(*ErrCASMismatch)
	return ok
}

// ============================================================================

// ============================================================================

type Domain[T any] struct {
	backend Backend
	prefix  string
}

func NewDomain[T any](backend Backend, prefix string) *Domain[T] {
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	return &Domain[T]{backend: backend, prefix: prefix}
}

func (d *Domain[T]) fullKey(key string) string {
	return d.prefix + key
}

func (d *Domain[T]) Get(ctx context.Context, key string) (T, uint64, error) {
	var zero T
	raw, version, err := d.backend.Get(ctx, d.fullKey(key))
	if err != nil {
		return zero, 0, err
	}
	if err := json.Unmarshal(raw, &zero); err != nil {
		return zero, version, fmt.Errorf("storage: unmarshal domain %q: %w", key, err)
	}
	return zero, version, nil
}

func (d *Domain[T]) Put(ctx context.Context, key string, value T, expectedVersion uint64) (uint64, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return 0, fmt.Errorf("storage: marshal domain %q: %w", key, err)
	}
	return d.backend.Put(ctx, d.fullKey(key), raw, expectedVersion)
}

func (d *Domain[T]) Delete(ctx context.Context, key string) error {
	return d.backend.Delete(ctx, d.fullKey(key))
}

func (d *Domain[T]) List(ctx context.Context) ([]string, error) {
	keys, err := d.backend.List(ctx, d.prefix)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, strings.TrimPrefix(k, d.prefix))
	}
	return out, nil
}

// ============================================================================

// ============================================================================

func Migrate(ctx context.Context, src, dst Backend) (migrated int, skipped []string, err error) {
	keys, err := src.List(ctx, "")
	if err != nil {
		return 0, nil, fmt.Errorf("storage: list src: %w", err)
	}
	for _, key := range keys {
		data, version, err := src.Get(ctx, key)
		if err != nil {
			if IsKeyNotFound(err) {
				continue
			}
			return migrated, skipped, fmt.Errorf("storage: get %q from src: %w", key, err)
		}

		_, dstVersion, dstErr := dst.Get(ctx, key)
		if dstErr == nil && dstVersion >= version {
			skipped = append(skipped, key)
			continue
		}

		if _, err := dst.Put(ctx, key, data, dstVersion); err != nil {
			return migrated, skipped, fmt.Errorf("storage: put %q to dst: %w", key, err)
		}
		migrated++
	}
	return migrated, skipped, nil
}
