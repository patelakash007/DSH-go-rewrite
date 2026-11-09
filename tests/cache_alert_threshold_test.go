//

package tests

import (
	"testing"

	"github.com/JopenChen/dsh-go/pkg/llm"
)

type collectAlerts struct {
	alerts []llm.Alert
}

func (c *collectAlerts) sink(a llm.Alert) { c.alerts = append(c.alerts, a) }

func TestN08SuddenDropWarns(t *testing.T) {
	col := &collectAlerts{}
	ca := llm.NewCacheAlert(col.sink)

	ca.Observe("s1", 0.95)

	ca.Observe("s1", 0.30)
	var sawWarn bool
	for _, a := range col.alerts {
		if a.Level == llm.AlertWarn {
			sawWarn = true
			if !containsStr(a.Message, "s1") || !containsStr(a.Message, "current") || !containsStr(a.Message, "previous") {
				t.Fatalf("warn should contain session/current/previous: %q", a.Message)
			}
		}
	}
	if !sawWarn {
		t.Fatal("sudden drop should generate warn alert")
	}
}

func TestN08ConsecutiveFailsSignalsError(t *testing.T) {
	col := &collectAlerts{}
	ca := llm.NewCacheAlert(col.sink)
	for i := 0; i < 6; i++ {
		ca.Observe("s2", 0.10)
	}
	var sawError bool
	for _, a := range col.alerts {
		if a.Level == llm.AlertError {
			sawError = true
			if !a.Webhook {
				t.Fatal("consecutive threshold breaches should trigger webhook")
			}
		}
	}
	if !sawError {
		t.Fatal("5 consecutive low hit rates should generate error + webhook")
	}
}

func TestN08RecoveryResets(t *testing.T) {
	col := &collectAlerts{}
	ca := llm.NewCacheAlert(col.sink)

	for i := 0; i < 3; i++ {
		ca.Observe("s3", 0.10)
	}
	ca.Observe("s3", 0.90)

	col.alerts = nil
	errCount := 0
	for i := 0; i < 4; i++ {
		ca.Observe("s3", 0.10)
	}
	for _, a := range col.alerts {
		if a.Level == llm.AlertError {
			errCount++
		}
	}
	if errCount != 0 {
		t.Fatalf("4 low rates after recovery should not trigger error (consecutive reset), got %d", errCount)
	}
}

func TestN08ConfigOverridable(t *testing.T) {
	col := &collectAlerts{}
	ca := &llm.CacheAlert{Threshold: 0.8, DropWarn: 0.1, ConsecutiveFails: 2, Sink: col.sink}

	ca.Observe("s4", 0.75)
	ca.Observe("s4", 0.75)
	var sawError bool
	for _, a := range col.alerts {
		if a.Level == llm.AlertError {
			sawError = true
		}
	}
	if !sawError {
		t.Fatal("with threshold=0.8/consecutive=2, two 0.75 rates should trigger error")
	}
}
