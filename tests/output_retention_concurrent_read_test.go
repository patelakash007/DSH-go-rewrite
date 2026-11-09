//

package tests

import (
	"bytes"
	"context"
	"sync"
	"testing"

	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/tools"
)

func TestOutputRetentionConcurrentReaders(t *testing.T) {
	ctx := context.Background()
	_ = ctx
	ret := tools.NewResultRetention()

	const size = 10 * 1024 * 1024
	big := make([]byte, size)
	for i := range big {
		big[i] = byte(i % 251)
	}
	callID := brand.NewToolCallID("tc-big")
	ret.Retain(callID.Raw(), big)

	var wg sync.WaitGroup
	results := make([][]byte, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			got, ok := ret.Read(callID.Raw())
			if !ok {
				results[idx] = nil
				return
			}
			results[idx] = got
		}(i)
	}
	wg.Wait()

	for i, got := range results {
		if got == nil {
			t.Fatalf("reader[%d] did not read result", i)
		}
		if len(got) != size {
			t.Fatalf("reader[%d] length %d != %d (missing)", i, len(got), size)
		}
		if !bytes.Equal(got, big) {
			t.Fatalf("reader[%d] content mismatch with original (truncated/corrupted)", i)
		}
	}

	results[0][0] = ^results[0][0]
	if results[1][0] == results[0][0] {
		t.Fatal("two readers should hold independent copies")
	}
}

func TestOutputRetentionRetainResult(t *testing.T) {
	ret := tools.NewResultRetention()
	callID := brand.NewToolCallID("tc-json")
	res := &tools.ToolCallResult{
		CallID:  callID,
		IsError: false,
		Value:   map[string]any{"ok": true, "lines": 3},
	}
	ret.RetainResult(res)

	got, ok := ret.Read(callID.Raw())
	if !ok {
		t.Fatal("should be readable after RetainResult")
	}

	if !bytes.HasPrefix(got, []byte("{")) || !bytes.Contains(got, []byte(`"lines":3`)) {
		t.Fatalf("retained value is expected JSON, got %s", got)
	}
}

func TestOutputRetentionString(t *testing.T) {
	ret := tools.NewResultRetention()
	ret.Retain("tc-str", []byte("beef"))
	got, ok := ret.Read("tc-str")
	if !ok || string(got) != "beef" {
		t.Fatalf("string should retain original text, got %q ok=%v", got, ok)
	}
}

func TestOutputRetentionMissing(t *testing.T) {
	ret := tools.NewResultRetention()
	if _, ok := ret.Read("nope"); ok {
		t.Fatal("unretained callID should return false")
	}

	ret.Retain("gone", []byte("x"))
	ret.Remove("gone")
	if _, ok := ret.Read("gone"); ok {
		t.Fatal("should return false after soft delete")
	}
}
