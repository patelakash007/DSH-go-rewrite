//

//

package sysprompt

// ============================================================================

// ============================================================================

type ChangeOnlyInjector struct {
	reg      *ContextRegistry
	lastHash string
	injects  int
}

func NewChangeOnlyInjector(reg *ContextRegistry, persistedHash string) *ChangeOnlyInjector {
	return &ChangeOnlyInjector{reg: reg, lastHash: persistedHash}
}

const ClearedContextText = "Current runtime context: none. Earlier runtime-context snapshots no longer apply."

func (in *ChangeOnlyInjector) MightInject() (text string, injected bool) {
	text, hash := in.reg.Compute()
	if hash == in.lastHash {
		return "", false
	}
	hadSnapshot := in.lastHash != ""
	in.lastHash = hash
	in.injects++

	if text == "" && hadSnapshot {
		return ClearedContextText, true
	}

	return text, true
}

func (in *ChangeOnlyInjector) InjectCount() int { return in.injects }

func (in *ChangeOnlyInjector) LastHash() string { return in.lastHash }

// ============================================================================

// ============================================================================

type contextSnapshot struct {
	Text string

	Hash string
}

type CompactPreserve struct {
	last contextSnapshot
}

func (c *CompactPreserve) Record(text, hash string) {
	c.last = contextSnapshot{Text: text, Hash: hash}
}

func (c *CompactPreserve) Latest() (text, hash string, ok bool) {
	if c.last.Hash == "" {
		return "", "", false
	}
	return c.last.Text, c.last.Hash, true
}
