//

package tests

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/JopenChen/dsh-go/internal/telemetry"
)

func TestN09MetricsNames(t *testing.T) {
	names := telemetry.MetricsNames()
	want := map[string]bool{
		"dsh.cache.hit_ratio":    false,
		"dsh.cache.hit_tokens":   false,
		"dsh.cache.miss_tokens":  false,
		"dsh.cache.broken_count": false,
	}
	for _, n := range names {
		if _, ok := want[n]; ok {
			want[n] = true
		}
	}
	for n, ok := range want {
		if !ok {
			t.Fatalf("metric %s missing", n)
		}
	}
}

func TestN09MetricsRecord(t *testing.T) {
	m := telemetry.NewCacheMetrics()
	m.Record(90, 10, 0.9)
	m.Record(80, 20, 0.8)
	if m.HitTokensTotal != 170 || m.MissTokensTotal != 30 {
		t.Fatalf("cumulative mismatch: hit=%d miss=%d", m.HitTokensTotal, m.MissTokensTotal)
	}
	m.MarkBroken()
	if m.BrokenCount != 1 {
		t.Fatalf("broken_count should be 1, got %d", m.BrokenCount)
	}
}

func TestN09HistogramP50(t *testing.T) {
	m := telemetry.NewCacheMetrics()

	m.Record(1, 0, 0.5)
	m.Record(1, 0, 0.6)
	m.Record(1, 0, 0.9)
	m.Record(1, 0, 1.0)
	if p := m.HistogramP50(); p != 0.9 {
		t.Fatalf("P50 should be 0.9, got %v", p)
	}
}

func TestN09GrafanaDashboardJSON(t *testing.T) {
	data, err := os.ReadFile("../deploy/grafana/dsh-cache-dashboard.json")
	if err != nil {
		t.Skipf("dashboard file missing: %v", err)
		return
	}
	var dash struct {
		Panels []struct {
			ID    int    `json:"id"`
			Type  string `json:"type"`
			Title string `json:"title"`
		} `json:"panels"`
	}
	if err := json.Unmarshal(data, &dash); err != nil {
		t.Fatalf("dashboard JSON invalid: %v", err)
	}
	if len(dash.Panels) != 3 {
		t.Fatalf("should have 3 panels, got %d", len(dash.Panels))
	}
}
