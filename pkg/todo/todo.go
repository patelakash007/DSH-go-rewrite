//

//

package todo

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/JopenChen/dsh-go/pkg/session"
	"github.com/JopenChen/dsh-go/pkg/tools"
)

type TodoStatus string

const (
	StatusPending    TodoStatus = "pending"
	StatusInProgress TodoStatus = "in_progress"
	StatusCompleted  TodoStatus = "completed"
)

func (s TodoStatus) Valid() bool {
	return s == StatusPending || s == StatusInProgress || s == StatusCompleted
}

type TodoItem struct {
	ID string `json:"id,omitempty"`

	Content string `json:"content"`

	Status TodoStatus `json:"status,omitempty"`

	Done bool `json:"done,omitempty"`
}

func (it TodoItem) ResolvedStatus() TodoStatus {
	if it.Status.Valid() {
		return it.Status
	}
	if it.Done {
		return StatusCompleted
	}
	return StatusPending
}

//

type TodoWriteTool struct {
	log *session.SessionLog

	AllowParallel bool
}

func NewTodoWriteTool(sl *session.SessionLog) *TodoWriteTool {
	return &TodoWriteTool{log: sl}
}

type todoWriteInput struct {
	Items []TodoItem `json:"items"`
}

func (t *TodoWriteTool) Name() string { return "todo_write" }

func (t *TodoWriteTool) Description() string { return "Replace the entire current todo list" }

func (t *TodoWriteTool) Execute(ctx context.Context, input map[string]any) (any, error) {
	raw, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("todo: marshal input: %w", err)
	}
	var in todoWriteInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, fmt.Errorf("todo: invalid input: %w", err)
	}

	items, err := Normalize(in.Items, t.AllowParallel)
	if err != nil {
		return nil, err
	}

	entries := make([]session.TodoEntry, len(items))
	contents := make([]string, len(items))
	for i, it := range items {
		entries[i] = session.TodoEntry{Content: it.Content, Status: string(it.ResolvedStatus())}
		contents[i] = it.Content
	}

	if _, err := t.log.Append(session.TodoWriteData{Items: contents, Entries: entries}); err != nil {
		return nil, fmt.Errorf("todo: append: %w", err)
	}
	c := Count(items)
	return map[string]any{
		"ok": true,
		"counts": map[string]int{
			"pending": c.Pending, "inProgress": c.InProgress, "completed": c.Completed,
		},
	}, nil
}

// TodoTool returns a *tools.Tool wrapper for integration with M23 pipeline.
func (t *TodoWriteTool) Tool() *tools.Tool {
	return &tools.Tool{
		Name:        t.Name(),
		Description: t.Description(),
		Execute:     t.Execute,
	}
}

func Current(log *session.SessionLog) []TodoItem {
	fold := session.FoldTodoWrite(log.Events())
	if len(fold.Entries) > 0 {
		out := make([]TodoItem, len(fold.Entries))
		for i, e := range fold.Entries {
			out[i] = TodoItem{Content: e.Content, Status: TodoStatus(e.Status)}
		}
		return out
	}
	out := make([]TodoItem, len(fold.Items))
	for i, c := range fold.Items {
		out[i] = TodoItem{Content: c, Status: StatusPending}
	}
	return out
}
