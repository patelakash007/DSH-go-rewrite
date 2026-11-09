//

package tests

import (
	"testing"

	"github.com/JopenChen/dsh-go/pkg/llm"
)

func TestCacheStatsHitRatioZero(t *testing.T) {
	cs := llm.CacheStats{}
	if r := cs.ComputeHitRatio(); r != 0.0 {
		t.Fatalf("total 0 hit rate should be 0.0, got %v", r)
	}
}

func TestCacheStatsHitRatioPartial(t *testing.T) {
	cs := llm.CacheStats{HitTokens: 79, MissTokens: 21}
	r := cs.ComputeHitRatio()
	if r < 0.789 || r > 0.791 {
		t.Fatalf("should be around 0.79, got %v", r)
	}
}

func TestCacheStatsHitRatioFull(t *testing.T) {
	cs := llm.CacheStats{HitTokens: 100, MissTokens: 0}
	if r := cs.ComputeHitRatio(); r != 1.0 {
		t.Fatalf("all hits should be 1.0, got %v", r)
	}
}

func TestCacheStatsHitRatioNone(t *testing.T) {
	cs := llm.CacheStats{HitTokens: 0, MissTokens: 50}
	if r := cs.ComputeHitRatio(); r != 0.0 {
		t.Fatalf("all misses should be 0.0, got %v", r)
	}
}

func TestCacheStatsAddRecord(t *testing.T) {
	cs := llm.CacheStats{}
	cs.RecordUsage(llm.Usage{PromptTokens: 1000, PromptCacheHitTokens: 800, PromptCacheMissTokens: 200})
	cs.RecordUsage(llm.Usage{PromptTokens: 500, PromptCacheHitTokens: 500, PromptCacheMissTokens: 0})
	if cs.HitTokens != 1300 || cs.MissTokens != 200 {
		t.Fatalf("cumulative error: %+v", cs)
	}
	r := cs.ComputeHitRatio() // 1300/1500 ≈ 0.8667
	if r < 0.86 || r > 0.87 {
		t.Fatalf("cumulative hit rate should be around 0.867, got %v", r)
	}

	line := llm.CacheLogLine(llm.Usage{PromptCacheHitTokens: 900, PromptCacheMissTokens: 100})
	if line != "cache.hit_ratio=0.900" {
		t.Fatalf("log line should have cache.hit_ratio=0.900, got %q", line)
	}
}
