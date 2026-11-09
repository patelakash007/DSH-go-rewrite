package todo

import (
	"errors"
	"strings"
)

var ErrEmptyContent = errors.New("todo: empty content")

var ErrDuplicateContent = errors.New("todo: duplicate content")

var ErrMultipleActive = errors.New("todo: more than one in_progress")

func Normalize(items []TodoItem, allowParallel bool) ([]TodoItem, error) {
	out := make([]TodoItem, 0, len(items))
	seen := map[string]struct{}{}
	active := 0
	for _, it := range items {
		content := strings.TrimSpace(it.Content)
		if content == "" {
			return nil, ErrEmptyContent
		}
		if _, dup := seen[content]; dup {
			return nil, ErrDuplicateContent
		}
		seen[content] = struct{}{}
		status := it.ResolvedStatus()
		if status == StatusInProgress {
			active++
		}
		out = append(out, TodoItem{ID: it.ID, Content: content, Status: status})
	}
	if !allowParallel && active > 1 {
		return nil, ErrMultipleActive
	}
	return out, nil
}

type Counts struct {
	Pending    int
	InProgress int
	Completed  int
}

func Count(items []TodoItem) Counts {
	var c Counts
	for _, it := range items {
		switch it.ResolvedStatus() {
		case StatusInProgress:
			c.InProgress++
		case StatusCompleted:
			c.Completed++
		default:
			c.Pending++
		}
	}
	return c
}
