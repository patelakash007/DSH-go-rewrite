package sessiontitle

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

func CleanTitle(input string) string {
	var b strings.Builder
	b.Grow(len(input))
	prevSpace := false
	leading := true
	runes := []rune(input)
	for i := 0; i < len(runes); {
		r := runes[i]

		if r == 0x1b {
			i = skipAnsi(runes, i)
			continue
		}
		switch {
		case isTitleControl(r):
			i++
			continue
		case unicode.IsSpace(r):
			if leading || prevSpace {
				i++
				continue
			}
			b.WriteRune(' ')
			prevSpace = true
			i++
			continue
		}
		b.WriteRune(r)
		prevSpace = false
		leading = false
		i++
	}
	return strings.TrimRight(b.String(), " ")
}

func skipAnsi(runes []rune, idx int) int {
	if idx+1 >= len(runes) {
		return len(runes)
	}
	switch runes[idx+1] {
	case '[':
		j := idx + 2
		for j < len(runes) && !(runes[j] >= 0x40 && runes[j] <= 0x7e) {
			j++
		}
		return j + 1
	case ']':
		j := idx + 2
		for j < len(runes) {
			if runes[j] == 0x07 {
				return j + 1
			}
			if runes[j] == 0x1b && j+1 < len(runes) && runes[j+1] == '\\' {
				return j + 2
			}
			j++
		}
		return j
	default:
		return idx + 2
	}
}

func isTitleControl(r rune) bool {
	if r == '\t' || r == '\n' || r == '\r' {
		return false
	}
	if r <= 0x1f || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
		return true
	}
	switch {
	case r == 0x200b, r == 0x200e, r == 0x200f:
		return true
	case r >= 0x202a && r <= 0x202e:
		return true
	case r >= 0x2060 && r <= 0x2064:
		return true
	case r >= 0x2066 && r <= 0x206f:
		return true
	case r == 0xfeff:
		return true
	}
	return false
}

func TruncateUTF8(input string, maxBytes int) string {
	if maxBytes <= 0 || len(input) <= maxBytes {
		if maxBytes <= 0 {
			return ""
		}
		return input
	}
	used := 0
	for _, r := range input {
		size := utf8.RuneLen(r)
		if used+size > maxBytes {
			break
		}
		used += size
	}
	return input[:used]
}

func Normalize(input string, maxBytes int) string {
	return strings.TrimRight(TruncateUTF8(CleanTitle(input), maxBytes), " ")
}
