//

//

package tools

import (
	"slices"
	"sync"
)

//

type Restriction struct {
	Allow []string `json:"allow,omitempty"`

	Deny []string `json:"deny,omitempty"`
}

func RestrictionAllow(names ...string) Restriction {
	return Restriction{Allow: names}
}

func RestrictionDeny(names ...string) Restriction {
	return Restriction{Deny: names}
}

type RestrictionLayer struct {
	Key string

	Mask Restriction
}

type RestrictionSet struct {
	mu     sync.RWMutex
	layers []RestrictionLayer
}

func NewRestrictionSet() *RestrictionSet {
	return &RestrictionSet{}
}

func (rs *RestrictionSet) Host(mask Restriction) {
	rs.SetLayer("host", mask)
}

func (rs *RestrictionSet) Scope(key string, mask Restriction) {
	rs.SetLayer(key, mask)
}

func (rs *RestrictionSet) SetLayer(key string, mask Restriction) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	for i := range rs.layers {
		if rs.layers[i].Key == key {
			rs.layers[i].Mask = mask
			return
		}
	}
	rs.layers = append(rs.layers, RestrictionLayer{Key: key, Mask: mask})
}

//

func (rs *RestrictionSet) Allowed(tool string) bool {
	if rs == nil {
		return true
	}
	rs.mu.RLock()
	layers := make([]RestrictionLayer, len(rs.layers))
	copy(layers, rs.layers)
	rs.mu.RUnlock()

	for i := len(layers) - 1; i >= 0; i-- {
		mask := layers[i].Mask
		if slices.Contains(mask.Allow, tool) {
			return true
		}
		if slices.Contains(mask.Deny, tool) {
			return false
		}
	}
	return true
}

func (rs *RestrictionSet) Denied(tool string) bool {
	return !rs.Allowed(tool)
}

func (rs *RestrictionSet) Filter(tools []*Tool) []*Tool {
	if rs == nil {
		return tools
	}
	out := make([]*Tool, 0, len(tools))
	for _, t := range tools {
		if t != nil && rs.Allowed(t.Name) {
			out = append(out, t)
		}
	}
	return out
}

func (rs *RestrictionSet) Layers() []RestrictionLayer {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	out := make([]RestrictionLayer, len(rs.layers))
	copy(out, rs.layers)
	return out
}
