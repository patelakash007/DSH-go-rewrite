//

//

package presets

import (
	"sort"
	"sync"

	"github.com/JopenChen/dsh-go/pkg/scope"
)

type StandingKey string

type AgentPreset struct {
	Key StandingKey

	Tools []string

	Prompt string
}

type PresetRegistry struct {
	mu        sync.RWMutex
	hostLayer *scope.Layer[*AgentPreset]
	layers    *scope.ScopedLayers[*AgentPreset]
}

func NewPresetRegistry() *PresetRegistry {
	host := scope.NewLayer[*AgentPreset](scope.Key("host"))
	return &PresetRegistry{
		hostLayer: host,
		layers:    scope.NewScopedLayers[*AgentPreset]().Push(host),
	}
}

func (r *PresetRegistry) Mount(p *AgentPreset) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hostLayer.Register(string(p.Key), p, 0)
}

func ComposeFrom(key StandingKey, bases ...*AgentPreset) *AgentPreset {
	toolSet := map[string]struct{}{}
	var prompt string
	for _, b := range bases {
		if b == nil {
			continue
		}
		for _, t := range b.Tools {
			toolSet[t] = struct{}{}
		}
		if prompt != "" {
			prompt += "\n"
		}
		prompt += b.Prompt
	}
	tools := make([]string, 0, len(toolSet))
	for t := range toolSet {
		tools = append(tools, t)
	}
	sort.Strings(tools)
	return &AgentPreset{Key: key, Tools: tools, Prompt: prompt}
}

func (r *PresetRegistry) Select(key StandingKey) (*AgentPreset, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.layers.Get(string(key))
}

func StandingKeyFor(key StandingKey) StandingKey {
	return key
}

func (r *PresetRegistry) Recompose(p *AgentPreset) {
	r.Mount(p)
}
