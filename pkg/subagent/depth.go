package subagent

import "errors"

var ErrInvalidDepth = errors.New("subagent: depth must be a non-negative integer")

func ChildDepth(parentDepth int) (int, error) {
	if parentDepth < 0 {
		return 0, ErrInvalidDepth
	}
	return parentDepth + 1, nil
}

func ResolveDepth(headerDepth, runtimeDepth int) (int, error) {
	if headerDepth < 0 || runtimeDepth < 0 {
		return 0, ErrInvalidDepth
	}
	if runtimeDepth > headerDepth {
		return runtimeDepth, nil
	}
	return headerDepth, nil
}

func CanDelegate(depth, maxDepth int) bool {
	if depth < 0 {
		return false
	}
	if maxDepth < 0 {
		return true
	}
	return depth < maxDepth
}
