package tokenmeter

import (
	"encoding/json"
	"math"

	"github.com/JopenChen/dsh-go/pkg/llm"
)

const (
	charsPerToken = 4
	blockOverhead = 4

	RoleOverhead = 4
)

func ceilDiv(n int) int {
	return int(math.Ceil(float64(n) / float64(charsPerToken)))
}

func EstimateContent(blocks []llm.ContentBlock) int {
	tokens := 0
	for _, b := range blocks {
		switch b.Kind {
		case llm.BlockText, llm.BlockReasoning:
			tokens += ceilDiv(len(b.Text)) + blockOverhead
		case llm.BlockToolUse:
			if b.ToolCall != nil {
				tokens += ceilDiv(len(b.ToolCall.Name))
				if raw, err := json.Marshal(b.ToolCall.Input); err == nil {
					tokens += ceilDiv(len(raw))
				}
			}
			tokens += blockOverhead
		case llm.BlockToolResult:
			if b.ToolResult != nil {

				tokens += ceilDiv(len(b.ToolResult.Content))
			}
			tokens += blockOverhead
		default:

			if raw, err := json.Marshal(b); err == nil {
				tokens += blockOverhead + ceilDiv(len(raw))
			} else {
				tokens += blockOverhead
			}
		}
	}
	return tokens
}

func EstimateMessage(msg llm.Message) int {
	return EstimateContent(msg.Content) + RoleOverhead
}

func EstimateMessages(msgs []llm.Message) int {
	total := 0
	for _, m := range msgs {
		total += EstimateMessage(m)
	}
	return total
}
