//

//

package sysprompt

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/JopenChen/dsh-go/pkg/tools"
)

// ============================================================================

// ============================================================================

const (
	SectionOrderPersona = 100

	SectionOrderPolicy = 200

	SectionOrderRuntimeContext = 300

	SectionOrderPlanPolicy = 500

	SectionOrderToolsSchema = 600

	SectionOrderSkillCatalog = 700
)

// ============================================================================
// Section & Assembler
// ============================================================================

type Section struct {
	Name string `json:"name"`

	Order int `json:"order"`

	Text string `json:"text"`
}

type Assembler struct {
	mu       sync.RWMutex
	sections []*Section
}

func New() *Assembler {
	return &Assembler{sections: []*Section{}}
}

func (a *Assembler) Register(name string, order int, text string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, s := range a.sections {
		if s.Name == name {
			s.Order = order
			s.Text = text
			return
		}
	}
	a.sections = append(a.sections, &Section{Name: name, Order: order, Text: text})
}

func (a *Assembler) Unregister(name string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i, s := range a.sections {
		if s.Name == name {
			a.sections = append(a.sections[:i], a.sections[i+1:]...)
			return
		}
	}
}

func (a *Assembler) Has(name string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	for _, s := range a.sections {
		if s.Name == name {
			return true
		}
	}
	return false
}

func (a *Assembler) Sections() []*Section {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make([]*Section, len(a.sections))
	copy(out, a.sections)

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Order != out[j].Order {
			return out[i].Order < out[j].Order
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func (a *Assembler) Assemble() string {
	parts := make([]string, 0, len(a.sections))
	for _, s := range a.Sections() {
		if strings.TrimSpace(s.Text) == "" {
			continue
		}
		parts = append(parts, s.Text)
	}
	return strings.Join(parts, "\n\n")
}

// ============================================================================

// ============================================================================

type toolsSchemaEntry struct {
	Type     string        `json:"type"`
	Function toolsFunction `json:"function"`
}

type toolsFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

func ToolsSectionText(toolList []*tools.Tool) (string, error) {
	if len(toolList) == 0 {
		return "", nil
	}

	sorted := append([]*tools.Tool(nil), toolList...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })

	entries := make([]toolsSchemaEntry, 0, len(sorted))
	for _, tl := range sorted {
		var params json.RawMessage
		if tl.Schema != nil {
			raw, err := tl.Schema.Marshal()
			if err != nil {
				return "", fmt.Errorf("sysprompt: marshal schema for tool %q: %w", tl.Name, err)
			}
			params = raw
		} else {
			params = json.RawMessage(`{"type":"object"}`)
		}
		entries = append(entries, toolsSchemaEntry{
			Type: "function",
			Function: toolsFunction{
				Name:        tl.Name,
				Description: tl.Description,
				Parameters:  params,
			},
		})
	}

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return "", fmt.Errorf("sysprompt: marshal tools schema: %w", err)
	}
	return "# Available Tools (Tools Schema):\n" + string(data), nil
}
