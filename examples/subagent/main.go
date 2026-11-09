//

//

//

//

//   go run ./examples/subagent
//

package main

import (
	"context"
	"fmt"

	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/subagent"
)

func main() {
	ctx := context.Background()

	parent := brand.NewSessionID("parent-task")

	rt := subagent.NewRuntime()

	// ------------------------------------------------------------------

	// ------------------------------------------------------------------
	fmt.Println("— Spawn subagent (in-process backend) —")
	for _, backend := range []string{"acp", "fork-copy"} {
		h, err := rt.Spawn(ctx, backend, subagent.SpawnRequest{
			Parent:    &parent,
			Input:     "Analyze requirements",
			MaxRounds: 3,
		})
		if err != nil {
			fmt.Printf("  [%s] spawn err: %v\n", backend, err)
			continue
		}
		if err := h.Drain(ctx); err != nil {
			fmt.Printf("  [%s] drain err: %v\n", backend, err)
			continue
		}

		parentStr := "(root)"
		if h.Lineage.Parent != nil {
			parentStr = h.Lineage.Parent.Raw()
		}
		fmt.Printf("  [%s] session=%s | lineage: parent=%s\n",
			backend, h.Lineage.Session.Raw(), parentStr)
	}

	// ------------------------------------------------------------------

	// ------------------------------------------------------------------
	for _, backend := range []string{"acp", "fork-copy"} {

		h, _ := rt.Spawn(ctx, backend, subagent.SpawnRequest{Parent: &parent, Input: "x"})
		_ = h.Drain(ctx)
	}
	before := rt.ActiveCount()
	rt.DisposeOwner(parent)
	fmt.Printf("\n— Parent dispose cascade: registered count %d after DisposeOwner —\n", before)

	// ------------------------------------------------------------------

	// ------------------------------------------------------------------
	if _, err := rt.Spawn(ctx, "does-not-exist", subagent.SpawnRequest{Input: "x"}); err != nil {
		if subagent.IsUnknownProvider(err) {
			fmt.Printf("— Unknown backend stable error: %v —\n", err)
		} else {
			fmt.Printf("— Other error: %v —\n", err)
		}
	}
}
