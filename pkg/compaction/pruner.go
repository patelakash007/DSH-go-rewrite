package compaction

const PruneMarker = "\n... [content omitted] ...\n"

func PruneText(text string, threshold, head, tail int) (string, bool) {
	r := []rune(text)
	if len(r) <= threshold {
		return text, false
	}
	if head < 0 {
		head = 0
	}
	if tail < 0 {
		tail = 0
	}
	if head+tail > len(r) {

		return text, false
	}
	return string(r[:head]) + PruneMarker + string(r[len(r)-tail:]), true
}
