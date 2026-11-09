//

//

package llm

import (
	"fmt"
	"sync"
)

type CacheStats struct {
	HitTokens int `json:"hitTokens"`

	MissTokens int `json:"missTokens"`
}

func (c *CacheStats) Add(hit, miss int) {
	c.HitTokens += hit
	c.MissTokens += miss
}

func (c *CacheStats) Total() int {
	return c.HitTokens + c.MissTokens
}

func (c *CacheStats) ComputeHitRatio() float64 {
	total := c.Total()
	if total <= 0 {
		return 0.0
	}
	return float64(c.HitTokens) / float64(total)
}

func HitRatioOf(u Usage) float64 {
	cs := CacheStats{HitTokens: u.PromptCacheHitTokens, MissTokens: u.PromptCacheMissTokens}
	return cs.ComputeHitRatio()
}

func CacheLogLine(u Usage) string {
	return fmt.Sprintf("cache.hit_ratio=%.3f", HitRatioOf(u))
}

func (c *CacheStats) RecordUsage(u Usage) {
	c.Add(u.PromptCacheHitTokens, u.PromptCacheMissTokens)
}

// ============================================================================

// ============================================================================

type AlertLevel string

const (
	AlertWarn  AlertLevel = "warn"
	AlertError AlertLevel = "error"
)

type Alert struct {
	Level AlertLevel `json:"level"`

	Message string `json:"message"`

	Webhook bool `json:"webhook,omitempty"`
}

type AlertSink func(a Alert)

//

type CacheAlert struct {
	Threshold float64

	DropWarn float64

	ConsecutiveFails int

	Sink AlertSink

	mu               sync.Mutex
	prevRatio        float64
	consecutiveFails int
	hasPrev          bool
	alerts           []Alert
}

func NewCacheAlert(sink AlertSink) *CacheAlert {
	return &CacheAlert{
		Threshold:        0.5,
		DropWarn:         0.3,
		ConsecutiveFails: 5,
		Sink:             sink,
	}
}

func (a *CacheAlert) Observe(sessionID string, ratio float64) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.hasPrev && a.prevRatio-ratio > a.DropWarn {
		alert := Alert{Level: AlertWarn,
			Message: "cache hit ratio dropped: session=" + sessionID +
				" current=" + ft(ratio) + " previous=" + ft(a.prevRatio)}
		a.emit(alert, false)
	}

	if ratio < a.Threshold {
		a.consecutiveFails++
		if a.consecutiveFails >= a.ConsecutiveFails {
			alert := Alert{Level: AlertError,
				Message: "cache hit ratio persistently low: session=" + sessionID +
					" consecutive=" + itoa2(a.consecutiveFails) + " ratio=" + ft(ratio)}
			a.emit(alert, true)
			a.consecutiveFails = 0
		}
	} else {
		a.consecutiveFails = 0
	}

	a.prevRatio = ratio
	a.hasPrev = true
}

func (a *CacheAlert) emit(al Alert, webhook bool) {
	al.Webhook = webhook
	a.alerts = append(a.alerts, al)
	if a.Sink != nil {
		a.Sink(al)
	}
}

func (a *CacheAlert) Alerts() []Alert {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]Alert(nil), a.alerts...)
}

func ft(f float64) string {
	return fmt.Sprintf("%.3f", f)
}

func itoa2(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
