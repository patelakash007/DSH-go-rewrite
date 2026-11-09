//

//

//

//   go run ./examples/todo
//

package main

import (
	"context"
	"fmt"

	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/session"
	"github.com/JopenChen/dsh-go/pkg/todo"
)

func main() {
	ctx := context.Background()

	sl := session.NewSessionLog(brand.NewSessionID("todo-demo"))

	tw := todo.NewTodoWriteTool(sl)

	// ------------------------------------------------------------------

	// ------------------------------------------------------------------
	if _, err := tw.Execute(ctx, map[string]any{
		"items": []any{
			map[string]any{"content": "Organize requirements", "done": false},
			map[string]any{"content": "Design interface", "done": false},
			map[string]any{"content": "Implement prototype", "done": true},
		},
	}); err != nil {
		panic(err)
	}
	fmt.Println("— After first write (3 items) —")
	dump(todo.Current(sl))

	// ------------------------------------------------------------------

	// ------------------------------------------------------------------
	if _, err := tw.Execute(ctx, map[string]any{
		"items": []any{
			map[string]any{"content": "Add tests", "done": false},
			map[string]any{"content": "Finalize docs", "done": false},
		},
	}); err != nil {
		panic(err)
	}
	fmt.Println("— After second write (2 items, replaced) —")
	dump(todo.Current(sl))

	fmt.Println("\n— Conclusion: Todo = replace-all list, derived from event log —")
}

func dump(items []todo.TodoItem) {
	for i, it := range items {
		fmt.Printf("  %d. [%s] %s\n", i+1, it.ResolvedStatus(), it.Content)
	}
}
