package session

import "errors"

type SeqRange struct {
	From int
	To   int
}

func strictlyIncreasing(values []int) bool {
	for i := 1; i < len(values); i++ {
		if values[i] <= values[i-1] {
			return false
		}
	}
	return true
}

func EncodeSeqRanges(values []int) []SeqRange {
	if !strictlyIncreasing(values) {
		out := make([]SeqRange, len(values))
		for i, v := range values {
			out[i] = SeqRange{v, v}
		}
		return out
	}
	var out []SeqRange
	for start := 0; start < len(values); {
		end := start
		for end+1 < len(values) && values[end+1] == values[end]+1 {
			end++
		}
		if end-start >= 2 {
			out = append(out, SeqRange{values[start], values[end]})
		} else {
			for i := start; i <= end; i++ {
				out = append(out, SeqRange{values[i], values[i]})
			}
		}
		start = end + 1
	}
	return out
}

func DecodeSeqRanges(ranges []SeqRange, maxEntries int) ([]int, error) {
	var out []int
	hadRange := false
	for _, r := range ranges {
		if r.From < 0 || r.To < r.From {
			return nil, errors.New("invalid seq range")
		}
		if r.To > r.From {
			hadRange = true
		}
		for v := r.From; v <= r.To; v++ {
			if maxEntries > 0 && len(out) >= maxEntries {
				return nil, errors.New("seq ranges exceed max entries")
			}
			out = append(out, v)
		}
	}
	if hadRange && !strictlyIncreasing(out) {
		return nil, errors.New("decoded seq ranges must be strictly increasing")
	}
	return out, nil
}
