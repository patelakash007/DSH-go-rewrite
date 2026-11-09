//

package compaction

import (
	"context"
	"sort"
	"strings"

	"github.com/JopenChen/dsh-go/pkg/session"
)

const BasicEngineSumm = "[Summary] Past conversation summary:"

type BasicEngine struct{}

func (b *BasicEngine) Summarize(ctx context.Context, events []session.SessionEvent, maxRunes int) (string, error) {
	_ = ctx

	type pair struct {
		seq uint64
		msg string
	}
	msgs := []pair{}

	cp := append([]session.SessionEvent(nil), events...)
	sort.SliceStable(cp, func(i, j int) bool { return cp[i].Seq < cp[j].Seq })
	for _, ev := range cp {
		if ev.Type == session.EventUserMessage {
			if d, ok := ev.Data.(session.UserMessageData); ok {
				msgs = append(msgs, pair{ev.Seq, d.Content})
			}
		}
	}

	const maxItems = 3
	var sb strings.Builder
	sb.WriteString(BasicEngineSumm)
	for i, m := range msgs {
		if i >= maxItems {
			break
		}
		sb.WriteString(m.msg)
		if i < maxItems-1 && i < len(msgs)-1 {
			sb.WriteString("；")
		}
	}
	return truncateRunes(sb.String(), maxRunes), nil
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}
