//

package subagent

import (
	"context"
	"sync"

	"github.com/JopenChen/dsh-go/pkg/brand"
)

type InProcessRunner func(ctx context.Context, req SpawnRequest) (sid brand.SessionID, output string, err error)

type InProcessProvider struct {
	runner InProcessRunner
	mu     sync.Mutex
}

func NewInProcessProvider(runner InProcessRunner) *InProcessProvider {
	return &InProcessProvider{runner: runner}
}

func (p *InProcessProvider) Name() string { return "in-process" }

func (p *InProcessProvider) Spawn(ctx context.Context, req SpawnRequest) (*Handle, error) {
	runner := p.runner
	if runner == nil {

		defaultRunner := func(_ context.Context, r SpawnRequest) (brand.SessionID, string, error) {
			sid := brand.NewSessionID("inprocess-" + r.Input)
			return sid, "in-process:" + r.Input, nil
		}
		runner = defaultRunner
	}

	sid, output, err := runner(ctx, req)
	h := newHandle("in-process", ForkLineage{Parent: req.Parent, Session: sid})
	if err != nil {

		h.markDrained()
		return h, err
	}
	_ = output

	h.markDrained()
	return h, nil
}
