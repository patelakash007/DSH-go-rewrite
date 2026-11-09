//

//

package tests

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/JopenChen/dsh-go/tests/testutil"
)

func buildPrompt(system, preset string, toolCount int, rounds int) string {
	var sb strings.Builder
	sb.WriteString("<system>You are a capable DeepSeek agent following workspace policy and plan mode. ")
	sb.WriteString(system)
	sb.WriteString("</system>\n<preset>")
	sb.WriteString(preset)
	sb.WriteString("</preset>\n")
	for i := 0; i < toolCount; i++ {
		sb.WriteString(fmt.Sprintf("[tool%d]{name: tool_%d, description: perform operation number %d on the current workspace}\n", i, i, i))
	}
	for i := 0; i < rounds; i++ {
		sb.WriteString(fmt.Sprintf("turn%d: user message text %d here\nassistant reply content %d here\n", i, i, i))
	}
	sb.WriteString("tail")
	return sb.String()
}

func avgAndMin(ratios []float64) (avg, min float64) {
	if len(ratios) == 0 {
		return 0, 0
	}
	sum := 0.0
	min = ratios[0]
	for _, r := range ratios {
		sum += r
		if r < min {
			min = r
		}
	}
	return sum / float64(len(ratios)), min
}

func TestN07E2E_T1_50TurnsStableHitRate(t *testing.T) {
	sim := testutil.NewPrefixCacheSimulator()

	for i := 0; i < 3; i++ {
		_, _ = sim.Simulate(buildPrompt("dsh", "default", 10, i))
	}
	var ratios []float64
	for i := 3; i < 53; i++ {
		hit, miss := sim.Simulate(buildPrompt("dsh", "default", 10, i))
		ratios = append(ratios, float64(hit)/float64(hit+miss))
	}
	avg, min := avgAndMin(ratios)
	t.Logf("T1 avg=%.4f min=%.4f", avg, min)
	if avg < 0.95 {
		t.Fatalf("steady hit rate over 50 rounds should be >= 0.95, got avg=%.4f", avg)
	}
}

func TestN07E2E_T2_PresetSwitch(t *testing.T) {
	sim := testutil.NewPrefixCacheSimulator()

	for i := 0; i < 20; i++ {
		_, _ = sim.Simulate(buildPrompt("dsh", "default", 10, i))
	}

	var post []float64
	for i := 0; i < 30; i++ {
		hit, miss := sim.Simulate(buildPrompt("dsh", "danger", 10, i))
		post = append(post, float64(hit)/float64(hit+miss))
	}
	first := post[0]
	last5 := post[25:30]
	avgLast, _ := avgAndMin(last5)
	t.Logf("T2 first-after-switch=%.4f last5=%.4f", first, avgLast)
	if first >= 0.5 {
		t.Fatalf("first request after switching preset should be < 0.5 (cache invalidation), got %.4f", first)
	}
	if avgLast < 0.8 {
		t.Fatalf("last 5 rounds after switching preset should be >= 0.8, got %.4f", avgLast)
	}
}

func TestN07E2E_T3_CompactionRecovery(t *testing.T) {
	sim := testutil.NewPrefixCacheSimulator()
	for i := 0; i < 50; i++ {
		_, _ = sim.Simulate(buildPrompt("dsh", "default", 10, i))
	}

	for i := 0; i < 3; i++ {
		_, _ = sim.Simulate(buildPrompt("dsh", "default", 10, i))
	}
	var post []float64
	for i := 3; i < 33; i++ {
		hit, miss := sim.Simulate(buildPrompt("dsh", "default", 10, i))
		post = append(post, float64(hit)/float64(hit+miss))
	}
	avg, _ := avgAndMin(post)
	t.Logf("T3 after compaction avg=%.4f", avg)
	if avg < 0.9 {
		t.Fatalf("30 rounds after compaction should recover near cache, got avg=%.4f", avg)
	}
}

func TestN07E2E_T4_MultiSession(t *testing.T) {
	const n = 10
	sims := make([]*testutil.PrefixCacheSimulator, n)
	for i := range sims {
		sims[i] = testutil.NewPrefixCacheSimulator()
	}
	var wg sync.WaitGroup
	avgs := make([]float64, n)
	for s := 0; s < n; s++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			var ratios []float64
			for i := 3; i < 33; i++ {
				hit, miss := sims[idx].Simulate(buildPrompt(fmt.Sprintf("worker%d", idx), "default", 8, i))
				ratios = append(ratios, float64(hit)/float64(hit+miss))
			}
			avgs[idx], _ = avgAndMin(ratios)
		}(s)
	}
	wg.Wait()
	for i, a := range avgs {
		t.Logf("session %d avg=%.4f", i, a)
		if a < 0.85 {
			t.Fatalf("session %d hit rate should be >= 0.85, got %.4f", i, a)
		}
	}
}

func TestN07E2E_T5_ToolCount(t *testing.T) {
	for _, tc := range []int{5, 10, 20} {
		sim := testutil.NewPrefixCacheSimulator()
		for i := 0; i < 3; i++ {
			_, _ = sim.Simulate(buildPrompt("dsh", "default", tc, i))
		}
		var ratios []float64
		for i := 3; i < 33; i++ {
			hit, miss := sim.Simulate(buildPrompt("dsh", "default", tc, i))
			ratios = append(ratios, float64(hit)/float64(hit+miss))
		}
		avg, _ := avgAndMin(ratios)
		t.Logf("tools=%d avg=%.4f", tc, avg)
		if avg < 0.95 {
			t.Fatalf("tool count %d hit rate should be >= 0.95, got %.4f", tc, avg)
		}
	}
}
