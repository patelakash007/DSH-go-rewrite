//

//

//   - dsh.cache.hit_tokens     (Counter)
//   - dsh.cache.miss_tokens    (Counter)

package telemetry

import "sync"

type CacheMetrics struct {
	mu              sync.Mutex
	HitRatios       []float64
	HitTokensTotal  int64
	MissTokensTotal int64
	BrokenCount     int64

	SessionID string
	Preset    string
	TurnSeq   int
}

func NewCacheMetrics() *CacheMetrics {
	return &CacheMetrics{}
}

func (m *CacheMetrics) Record(hit, miss int, ratio float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.HitRatios = append(m.HitRatios, ratio)
	m.HitTokensTotal += int64(hit)
	m.MissTokensTotal += int64(miss)
}

func (m *CacheMetrics) MarkBroken() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.BrokenCount++
}

func (m *CacheMetrics) HistogramP50() float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.HitRatios) == 0 {
		return 0
	}
	sorted := append([]float64(nil), m.HitRatios...)

	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j-1] > sorted[j]; j-- {
			sorted[j-1], sorted[j] = sorted[j], sorted[j-1]
		}
	}
	idx := len(sorted) / 2
	return sorted[idx]
}

func MetricsNames() []string {
	return []string{
		"dsh.cache.hit_ratio",
		"dsh.cache.hit_tokens",
		"dsh.cache.miss_tokens",
		"dsh.cache.broken_count",
	}
}
