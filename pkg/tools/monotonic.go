//

//

package tools

import (
	"errors"
)

var ErrGuardRelaxed = errors.New("tools monotonic guard: cannot relax an already stricter decision")

func guardRank(d PreToolDecision) int {
	switch d {
	case PreDeny:
		return 2
	case PreAsk:
		return 1
	default: // PreAllow
		return 0
	}
}

type MonotonicGuard struct {
	current PreToolDecision
	settled bool
}

func NewMonotonicGuard() *MonotonicGuard {
	return &MonotonicGuard{current: PreAsk}
}

//

func (g *MonotonicGuard) Update(next PreToolDecision) error {
	if !g.settled {
		g.current = next
		g.settled = true
		return nil
	}
	if guardRank(next) < guardRank(g.current) {
		return ErrGuardRelaxed
	}
	g.current = next
	return nil
}

func (g *MonotonicGuard) Decision() PreToolDecision {
	if !g.settled {
		return PreAsk
	}
	return g.current
}

func (g *MonotonicGuard) IsDenied() bool {
	return g.settled && g.current == PreDeny
}
