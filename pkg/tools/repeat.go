package tools

import (
	"encoding/json"
	"sort"
	"strings"
)

var DefaultRepeatThresholds = []int{3, 5, 8}

func canonicalValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := map[string]any{}
		for _, k := range keys {
			out[k] = canonicalValue(t[k])
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i := range t {
			out[i] = canonicalValue(t[i])
		}
		return out
	default:
		return v
	}
}

func CanonicalizeArgs(input map[string]any) string {
	b, err := json.Marshal(canonicalValue(input))
	if err != nil {
		return ""
	}
	return string(b)
}

type RepeatState struct {
	lastKey string
	count   int
}

func (r *RepeatState) Observe(toolName string, args map[string]any) int {
	key := toolName + "\x00" + CanonicalizeArgs(args)
	if key == r.lastKey {
		r.count++
	} else {
		r.lastKey = key
		r.count = 1
	}
	return r.count
}

func (r *RepeatState) Reset() {
	r.lastKey = ""
	r.count = 0
}

func HitThreshold(count int, thresholds []int) bool {
	for _, th := range thresholds {
		if count == th {
			return true
		}
	}
	return false
}

const GentleReminder = "You are repeating the same tool call with identical arguments. Please carefully analyze the previous result first; " +
	"if the task is not completed, try a different approach or different parameters instead of repeating the call."

func DetailedReminder(toolName string, count int, canonical string, previewCap int) string {
	preview := canonical
	if previewCap > 0 && len(preview) > previewCap {
		preview = string([]rune(preview)[:previewCap]) + "…"
	}
	var b strings.Builder
	b.WriteString("Repeated tool call detected:\n")
	b.WriteString("- Tool: " + toolName + "\n")
	b.WriteString("- Consecutive count: ")
	b.WriteString(itoaRepeat(count))
	b.WriteString("\n- Arguments: " + preview + "\n")
	b.WriteString("These repetitions made no progress. Do not call this tool again with identical arguments.")
	return b.String()
}

func itoaRepeat(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
