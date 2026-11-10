//

//

//   - Skill{name/description/rank/modelInvocable/userInvocable/content}；

package skills

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// ============================================================================

// ============================================================================

type Skill struct {
	Name string

	Description string

	Path string

	Rank int

	ModelInvocable bool

	UserInvocable bool

	Content string
}

type SkillCandidate struct {
	Rank int

	Locator string
}

type SkillLayer struct {
	Title string

	Dir string

	Rank int
}

func defaultLayers(root string) []SkillLayer {
	home, _ := os.UserHomeDir()
	return []SkillLayer{
		{Title: "project-dsh", Dir: filepath.Join(root, ".dsh", "skills"), Rank: 0},
		{Title: "project-agents", Dir: filepath.Join(root, ".agents", "skills"), Rank: 1},
		{Title: "custom", Dir: filepath.Join(root, ".config", "skills"), Rank: 2},
		{Title: "user-dsh", Dir: filepath.Join(home, ".dsh", "skills"), Rank: 3},
		{Title: "user-agents", Dir: filepath.Join(home, ".agents", "skills"), Rank: 4},
		{Title: "bundled", Dir: filepath.Join("internal", "skills"), Rank: 5},
	}
}

type SkillRegistry struct {
	mu     sync.RWMutex
	layers []SkillLayer
	skills map[string]*Skill
}

func New(root string) *SkillRegistry {
	return &SkillRegistry{
		layers: defaultLayers(root),
		skills: map[string]*Skill{},
	}
}

func (r *SkillRegistry) AddLayer(l SkillLayer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.layers = append(r.layers, l)
}

func (r *SkillRegistry) Layers() []SkillLayer {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]SkillLayer{}, r.layers...)
}

func (r *SkillRegistry) Scan() {
	r.mu.Lock()
	defer r.mu.Unlock()
	merged := map[string]*Skill{}

	layers := append([]SkillLayer{}, r.layers...)

	sort.SliceStable(layers, func(i, j int) bool { return layers[i].Rank < layers[j].Rank })
	for _, layer := range layers {
		for _, skill := range scanDir(layer.Dir, layer.Rank) {
			if existing, ok := merged[skill.Name]; ok && existing.Rank < skill.Rank {

				continue
			}
			merged[skill.Name] = skill
		}
	}
	r.skills = merged
}

func (r *SkillRegistry) List() []*Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.skills))
	for n := range r.skills {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]*Skill, 0, len(names))
	for _, n := range names {
		out = append(out, r.skills[n])
	}
	return out
}

func (r *SkillRegistry) Get(name string) (*Skill, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.skills[name]
	return s, ok
}

func (r *SkillRegistry) Candidates() []SkillCandidate {
	list := r.List()
	out := make([]SkillCandidate, 0, len(list))
	for _, s := range list {
		out = append(out, SkillCandidate{Rank: s.Rank, Locator: s.Path})
	}
	return out
}

func scanDir(dir string, rank int) []*Skill {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []*Skill
	for _, e := range entries {
		name := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
		path := filepath.Join(dir, e.Name())
		if e.IsDir() {
			path = filepath.Join(path, "SKILL.md")
			name = e.Name()
		} else if !strings.HasSuffix(strings.ToLower(e.Name()), ".md") {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		s := parseSkill(name, path, string(data), rank)
		if s != nil {
			out = append(out, s)
		}
	}
	return out
}

func parseSkill(name, path, text string, rank int) *Skill {
	lines := strings.Split(text, "\n")
	desc := ""
	modelInv := true
	userInv := true
	var bodyLines []string
	inFrontmatter := len(lines) > 0 && strings.TrimSpace(lines[0]) == "---"
	frontmatterClosed := !inFrontmatter
	for index, ln := range lines {
		if index == 0 && inFrontmatter {
			continue
		}
		trim := strings.TrimSpace(ln)
		low := strings.ToLower(trim)
		if inFrontmatter && !frontmatterClosed {
			if trim == "---" {
				frontmatterClosed = true
				continue
			}
			parts := strings.SplitN(trim, ":", 2)
			if len(parts) == 2 {
				key := strings.ToLower(strings.TrimSpace(parts[0]))
				value := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
				switch key {
				case "name":
					if value != "" {
						name = value
					}
				case "description":
					desc = value
				case "model-invocable":
					modelInv = !strings.EqualFold(value, "false")
				case "user-invocable":
					userInv = !strings.EqualFold(value, "false")
				case "disable-model-invocation":
					modelInv = !strings.EqualFold(value, "true")
				case "disable-user-invocation":
					userInv = !strings.EqualFold(value, "true")
				}
			}
			continue
		}
		if strings.HasPrefix(low, "description:") || strings.HasPrefix(low, "**description**:") {
			parts := strings.SplitN(trim, ":", 2)
			if len(parts) == 2 {
				desc = strings.TrimSpace(parts[1])
			}
			continue
		}
		if strings.Contains(low, "model-invocable: false") || strings.Contains(low, "disable-model-invocation: true") {
			modelInv = false
			continue
		}
		if strings.Contains(low, "user-invocable: false") || strings.Contains(low, "disable-user-invocation: true") {
			userInv = false
			continue
		}
		bodyLines = append(bodyLines, ln)
	}
	if desc == "" && len(bodyLines) > 0 {

		for _, ln := range bodyLines {
			if strings.TrimSpace(ln) != "" {
				desc = strings.TrimSpace(ln)
				break
			}
		}
	}
	return &Skill{
		Name:           name,
		Description:    desc,
		Path:           path,
		Rank:           rank,
		ModelInvocable: modelInv,
		UserInvocable:  userInv,
		Content:        strings.Join(bodyLines, "\n"),
	}
}

func (r *SkillRegistry) Hash() string {
	list := r.List()
	if len(list) == 0 {
		return "empty"
	}
	var sb strings.Builder
	for _, s := range list {
		sb.WriteString(fmt.Sprintf("%s@%d:%d:", s.Name, s.Rank, len(s.Content)))
	}

	return sb.String()
}

// ============================================================================

// ============================================================================

func (r *SkillRegistry) CatalogText() string {
	list := r.List()
	if len(list) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("<available_skills>\n")
	for _, s := range list {
		sb.WriteString("- ")
		sb.WriteString(s.Name)
		sb.WriteString(" (rank ")
		sb.WriteString(fmt.Sprint(s.Rank))
		sb.WriteString("): ")
		sb.WriteString(strings.TrimSpace(s.Description))
		sb.WriteString("\n")
	}
	sb.WriteString("</available_skills>")
	return sb.String()
}

func (r *SkillRegistry) CatalogHash() string {
	return catalogHashOf(r.CatalogText())
}

func catalogHashOf(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

type Injector struct {
	registry *SkillRegistry
	lastHash string
	injects  int
}

func NewInjector(r *SkillRegistry) *Injector {
	return &Injector{registry: r, lastHash: ""}
}

func (in *Injector) MaybeInject() (content string, injected bool) {
	hash := in.registry.CatalogHash()
	if hash == in.lastHash {
		return "", false
	}
	in.lastHash = hash
	in.injects++
	return in.registry.CatalogText(), true
}

func (in *Injector) InjectCount() int { return in.injects }

// ============================================================================

// ============================================================================

func (r *SkillRegistry) Watch(stop <-chan struct{}, interval time.Duration, onChange func(names []string)) {
	if interval <= 0 {
		interval = time.Second
	}
	r.Scan()
	cur := r.Hash()
	for {
		select {
		case <-stop:
			return
		case <-time.After(interval):
			r.Scan()
			newHash := r.Hash()
			if newHash != cur && onChange != nil {
				onChange(currentNames(r))
			}
			cur = newHash
		}
	}
}

func currentNames(r *SkillRegistry) []string {
	list := r.List()
	names := make([]string, 0, len(list))
	for _, s := range list {
		names = append(names, s.Name)
	}
	return names
}

// ============================================================================
// tool-skill
// ============================================================================

type SkillTool struct {
	registry *SkillRegistry
}

func NewSkillTool(r *SkillRegistry) *SkillTool { return &SkillTool{registry: r} }

func (t *SkillTool) Resolve(name string, byModel bool) (string, error) {
	s, ok := t.registry.Get(name)
	if !ok {
		return "", fmt.Errorf("skills: skill %q not found", name)
	}
	if byModel && !s.ModelInvocable {
		return "", fmt.Errorf("skills: %q is not model-invocable", name)
	}
	if !byModel && !s.UserInvocable {
		return "", fmt.Errorf("skills: %q is not user-invocable", name)
	}
	return s.Content, nil
}
