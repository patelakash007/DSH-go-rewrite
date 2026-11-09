//

//

package workflow

import (
	"context"
	"fmt"
	"sync"
)

// ============================================================================

// ============================================================================

type StepRunFunc func(ctx context.Context) (string, error)

type Step struct {
	Name string
	Run  StepRunFunc
}

type Result struct {
	Name      string   `json:"name"`
	Outputs   []string `json:"outputs"`
	Cancelled bool     `json:"cancelled"`
}

func (r *Result) Errors() error {
	for _, o := range r.Outputs {
		if s, ok := unwrapErr(o); ok {
			return s
		}
	}
	return nil
}

func unwrapErr(o string) (error, bool) {
	return nil, false
}

// ============================================================================
// Workflow
// ============================================================================

type Mode string

const (
	ModePipeline Mode = "pipeline"
	ModeParallel Mode = "parallel"
)

type Workflow struct {
	Name  string
	Mode  Mode
	Steps []*Step
}

// ============================================================================

// ============================================================================

func Pipeline(name string, steps ...*Step) *Workflow {
	return &Workflow{Name: name, Mode: ModePipeline, Steps: steps}
}

func Parallel(name string, steps ...*Step) *Workflow {
	return &Workflow{Name: name, Mode: ModeParallel, Steps: steps}
}

// "in-process"/"acp"/"fork-copy"）。
func Agent(name, backendName, input string, spawn SpawnAgentFunc) *Step {
	return &Step{
		Name: name,
		Run: func(ctx context.Context) (string, error) {
			return spawn(ctx, backendName, input)
		},
	}
}

func Seq(w *Workflow) *Step {
	return &Step{
		Name: w.Name,
		Run: func(ctx context.Context) (string, error) {
			res, err := w.Run(ctx)
			if err != nil {
				return "", err
			}
			out := ""
			for i, o := range res.Outputs {
				if i > 0 {
					out += ","
				}
				out += o
			}
			return out, nil
		},
	}
}

type SpawnAgentFunc func(ctx context.Context, backendName, input string) (string, error)

// ============================================================================
// Run
// ============================================================================

func (w *Workflow) Run(ctx context.Context) (*Result, error) {
	res := &Result{Name: w.Name}
	switch w.Mode {
	case ModeParallel:
		return w.runParallel(ctx, res)
	default: // ModePipeline
		return w.runPipeline(ctx, res)
	}
}

func (w *Workflow) runPipeline(ctx context.Context, res *Result) (*Result, error) {
	for _, s := range w.Steps {
		if err := ctx.Err(); err != nil {
			res.Cancelled = true
			return res, err
		}
		out, err := s.Run(ctx)
		if err == nil {
			res.Outputs = append(res.Outputs, out)
		} else {
			res.Outputs = append(res.Outputs, fmt.Sprintf("[%s:error %v]", s.Name, err))
		}

		if err != nil {
			return res, err
		}
	}
	return res, nil
}

func (w *Workflow) runParallel(ctx context.Context, res *Result) (*Result, error) {
	outputs := make([]string, len(w.Steps))
	var wg sync.WaitGroup
	for i, s := range w.Steps {
		wg.Add(1)
		go func(idx int, step *Step) {
			defer wg.Done()
			out, err := step.Run(ctx)
			if err != nil {
				outputs[idx] = fmt.Sprintf("[%s:error %v]", step.Name, err)
				return
			}
			outputs[idx] = out
		}(i, s)
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		res.Cancelled = true
	}
	res.Outputs = outputs
	return res, nil
}
