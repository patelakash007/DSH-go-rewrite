//

//

package compaction

import (
	"context"
	"fmt"
	"time"

	"github.com/JopenChen/dsh-go/pkg/session"
)

// ============================================================================

// ============================================================================

const defaultMaxTokens = 20000

const defaultKeepTail = 10

const defaultSummaryMaxRunes = 800

type Config struct {
	MaxTokens       int
	KeepTail        int
	SummaryMaxRunes int
}

func (c Config) WithDefaults() Config {
	if c.MaxTokens <= 0 {
		c.MaxTokens = defaultMaxTokens
	}
	if c.KeepTail <= 0 {
		c.KeepTail = defaultKeepTail
	}
	if c.SummaryMaxRunes <= 0 {
		c.SummaryMaxRunes = defaultSummaryMaxRunes
	}
	return c
}

// ============================================================================

// ============================================================================

type Summarizer interface {
	Summarize(ctx context.Context, events []session.SessionEvent, maxRunes int) (string, error)
}

// ============================================================================

// ============================================================================

func tokenEstimate(ev session.SessionEvent) int {
	var text string
	switch d := ev.Data.(type) {
	case session.UserMessageData:
		text = d.Content
	case session.AssistantMessageData:
		text = d.Content
	case session.SessionTitleData:
		text = d.Title
	case session.InjectionContextData:
		text = d.Content
	default:
		return 0
	}
	runes := len([]rune(text))
	return runes/2 + 1
}

func EstimateTokens(events []session.SessionEvent) int {
	total := 0
	for _, ev := range events {
		total += tokenEstimate(ev)
	}
	return total
}

func ShouldCompact(events []session.SessionEvent, maxTokens int) bool {
	if maxTokens <= 0 {
		return false
	}
	return EstimateTokens(events) > maxTokens
}

// ============================================================================
// Result
// ============================================================================

type Result struct {
	Start       uint64
	End         uint64
	Summary     string
	Replacement session.SessionEvent

	Effective []session.SessionEvent
}

// ============================================================================
// Compactor
// ============================================================================

type Compactor struct {
	cfg        Config
	summarizer Summarizer
}

func New(cfg Config, summarizer Summarizer) *Compactor {
	cfg = cfg.WithDefaults()
	if summarizer == nil {
		summarizer = &BasicEngine{}
	}
	return &Compactor{cfg: cfg, summarizer: summarizer}
}

func (c *Compactor) ShouldCompact(events []session.SessionEvent) bool {
	return ShouldCompact(events, c.cfg.MaxTokens)
}

func (c *Compactor) Compact(ctx context.Context, events []session.SessionEvent) (Result, error) {
	if len(events) == 0 {
		return Result{}, fmt.Errorf("compaction: empty events")
	}
	if !c.ShouldCompact(events) {
		return Result{}, fmt.Errorf("compaction: below token threshold, no need to compact")
	}

	keepTail := c.cfg.KeepTail * 2
	if keepTail >= len(events) {
		keepTail = len(events) - 1
		if keepTail < 1 {
			keepTail = 0
		}
	}
	keepIndex := len(events) - keepTail

	start := events[0].Seq
	end := events[keepIndex-1].Seq
	if keepIndex == 0 {

		end = events[len(events)-1].Seq
	}

	oldForSummary := events[:keepIndex]
	summary, err := c.summarizer.Summarize(ctx, oldForSummary, c.cfg.SummaryMaxRunes)
	if err != nil {
		return Result{}, err
	}

	runes := []rune(summary)
	if len(runes) > c.cfg.SummaryMaxRunes {
		summary = string(runes[:c.cfg.SummaryMaxRunes])
	}

	lastSeq := events[len(events)-1].Seq
	replacement := session.SessionEvent{
		Seq:       lastSeq + 1,
		Time:      time.Now(),
		Type:      session.EventAssistantMessage,
		Data:      session.AssistantMessageData{Content: summary},
		SurfaceOp: session.NewReplaceOp(start, end, nil),
	}

	effective := append(append([]session.SessionEvent(nil), events...), replacement)

	return Result{
		Start:       start,
		End:         end,
		Summary:     summary,
		Replacement: replacement,
		Effective:   effective,
	}, nil
}
