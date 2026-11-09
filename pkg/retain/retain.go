package retain

type Result[T any] struct {
	Items     []T
	Seen      int
	Omitted   int
	Truncated bool
}

type ItemRetainer[T any] struct {
	maxItems int
	items    []T
	seen     int
	omitted  int
}

func NewItemRetainer[T any](maxItems int) *ItemRetainer[T] {
	if maxItems < 0 {
		maxItems = 0
	}
	return &ItemRetainer[T]{maxItems: maxItems}
}

func (r *ItemRetainer[T]) Push(item T) bool {
	r.seen++
	if len(r.items) < r.maxItems {
		r.items = append(r.items, item)
		return true
	}
	r.omitted++
	return false
}

func (r *ItemRetainer[T]) Finish() Result[T] {
	return Result[T]{
		Items:     r.items,
		Seen:      r.seen,
		Omitted:   r.omitted,
		Truncated: r.omitted > 0,
	}
}
