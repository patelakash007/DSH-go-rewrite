package tests

import (
	"context"
	"reflect"
	"testing"

	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/session"
	"github.com/JopenChen/dsh-go/pkg/todo"
)

func TestTodoWriteReplace(t *testing.T) {
	sl := session.NewSessionLog(brand.NewSessionID("todo_1"))
	tool := todo.NewTodoWriteTool(sl)

	_, err := tool.Execute(context.Background(), map[string]any{
		"items": []any{
			map[string]any{"content": "a"},
			map[string]any{"content": "b"},
		},
	})
	if err != nil {
		t.Fatalf("first execution failed: %v", err)
	}
	if got := contentsOf(todo.Current(sl)); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("first replacement result = %v, want [a b]", got)
	}

	_, err = tool.Execute(context.Background(), map[string]any{
		"items": []any{
			map[string]any{"content": "c"},
		},
	})
	if err != nil {
		t.Fatalf("second execution failed: %v", err)
	}
	if got := contentsOf(todo.Current(sl)); !reflect.DeepEqual(got, []string{"c"}) {
		t.Fatalf("second call should replace entirely with [c], got %v", got)
	}

	_, err = tool.Execute(context.Background(), map[string]any{"items": []any{}})
	if err != nil {
		t.Fatalf("clear execution failed: %v", err)
	}
	if got := todo.Current(sl); len(got) != 0 {
		t.Fatalf("should be empty after clearing, got %v", got)
	}

	fold := session.FoldTodoWrite(sl.Events())
	if !fold.Present || len(fold.Items) != 0 {
		t.Fatalf("fold should reflect latest empty list: %+v", fold)
	}
}

func contentsOf(items []todo.TodoItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Content
	}
	return out
}
