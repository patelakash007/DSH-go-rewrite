//

//

//

//   go run ./examples/workflow
//

package main

import (
	"context"
	"fmt"
	"time"

	"github.com/JopenChen/dsh-go/pkg/workflow"
)

func main() {
	ctx := context.Background()

	// ------------------------------------------------------------------

	// ------------------------------------------------------------------
	pipe := workflow.Pipeline("prepare",
		step("Fetch code", 50*time.Millisecond, "repo fetched"),
		step("Build", 60*time.Millisecond, "build ok"),
		step("Run tests", 30*time.Millisecond, "tests pass"),
	)
	res, err := pipe.Run(ctx)
	if err != nil {
		fmt.Printf("Pipeline failed: %v\n", err)
	} else {
		fmt.Println("— Sequential Pipeline Output —")
		for _, o := range res.Outputs {
			fmt.Printf("  · %s\n", o)
		}
	}

	// ------------------------------------------------------------------

	// ------------------------------------------------------------------
	par := workflow.Parallel("fetch-3",
		step("Request A", 120*time.Millisecond, "data A"),
		step("Request B", 90*time.Millisecond, "data B"),
		step("Request C", 150*time.Millisecond, "data C"),
	)
	start := time.Now()
	pres, _ := par.Run(ctx)
	fmt.Printf("\n— Parallel Output —\n")
	for _, o := range pres.Outputs {
		fmt.Printf("  · %s\n", o)
	}
	fmt.Printf("  (Parallel duration: %v)\n", time.Since(start).Round(time.Millisecond))

	// ------------------------------------------------------------------

	// ------------------------------------------------------------------
	agentFlow := workflow.Pipeline("with-subagent",
		step("Prepare input", 20*time.Millisecond, "input-ready"),
		workflow.Agent("Review", "in-process", "Review code request", func(_ context.Context, _ string, input string) (string, error) {

			return "subagent review completed: " + input, nil
		}),
	)
	ares, _ := agentFlow.Run(ctx)
	fmt.Printf("\n— Workflow with Subagent Step —\n")
	for _, o := range ares.Outputs {
		fmt.Printf("  · %s\n", o)
	}

	// ------------------------------------------------------------------

	// ------------------------------------------------------------------
	ctx2, cancel := context.WithCancel(context.Background())
	cancelAfter := workflow.Parallel("cancel-demo",
		step("Short task", 40*time.Millisecond, "short done"),
		step("Long task", 800*time.Millisecond, "long done"),
	)
	go func() { time.Sleep(60 * time.Millisecond); cancel() }()
	cres, cerr := cancelAfter.Run(ctx2)

	fmt.Printf("\n— Context cancellation cascade demo —\n  cancelled=%v, cnt=%d, err=%v\n",
		cres.Cancelled, len(cres.Outputs), cerr)
}

func step(name string, dur time.Duration, out string) *workflow.Step {
	return &workflow.Step{
		Name: name,
		Run: func(c context.Context) (string, error) {
			select {
			case <-time.After(dur):
				return out, nil
			case <-c.Done():
				return "", c.Err()
			}
		},
	}
}
