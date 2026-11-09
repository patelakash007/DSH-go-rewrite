package testutil

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
)

type MockChatRequest struct {
	Messages []struct {
		Role    string `json:"role"`
		Content any    `json:"content"`
	} `json:"messages"`
}

type PrefixHash struct {
	PromptText string

	Tokens int
}

type PrefixCacheSimulator struct {
	mu         sync.Mutex
	lastPrompt string
}

func NewPrefixCacheSimulator() *PrefixCacheSimulator {
	return &PrefixCacheSimulator{}
}

func LongestCommonPrefix(a, b string) int {
	n := 0
	for n < len(a) && n < len(b) && a[n] == b[n] {
		n++
	}
	return n
}

func (s *PrefixCacheSimulator) Simulate(prompt string) (hit, miss int) {
	tok := len(prompt) / 4
	if tok < 1 {
		tok = 1
	}
	s.mu.Lock()
	last := s.lastPrompt
	s.lastPrompt = prompt
	s.mu.Unlock()
	common := LongestCommonPrefix(prompt, last)
	hitFrac := float64(common) / float64(len(prompt))
	if len(prompt) == 0 {
		hitFrac = 0
	}
	hit = int(float64(tok) * hitFrac)
	miss = tok - hit
	return hit, miss
}

func NewDeepSeekMock() (*httptest.Server, *PrefixCacheSimulator) {
	sim := NewPrefixCacheSimulator()
	var mu sync.Mutex
	var reports []float64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req MockChatRequest
		_ = json.NewDecoder(r.Body).Decode(&req)

		promptText := ""
		for _, m := range req.Messages {
			switch c := m.Content.(type) {
			case string:
				promptText += c + "\n"
			case []any:
				for _, blk := range c {
					if s, ok := blk.(string); ok {
						promptText += s + "\n"
					}
				}
			}
		}
		hit, miss := sim.Simulate(promptText)
		ratio := float64(hit) / float64(hit+miss)
		if hit+miss == 0 {
			ratio = 0
		}
		mu.Lock()
		reports = append(reports, ratio)
		mu.Unlock()

		body := `data: {"choices":[],"usage":{"prompt_tokens":` + itoa(hit+miss) +
			`,"completion_tokens":5,"prompt_cache_hit_tokens":` + itoa(hit) +
			`,"prompt_cache_miss_tokens":` + itoa(miss) + `}}` + "\n\ndata: [DONE]\n\n"
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(body))
	}))
	return srv, sim
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
