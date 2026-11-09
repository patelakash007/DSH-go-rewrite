package fileref

import "regexp"

type Token struct {
	Prefix string
	Query  string
	Quoted bool
}

var (
	reQuoted = regexp.MustCompile(`(?:^|\s)(@"([^"]*))$`)
	rePlain  = regexp.MustCompile(`(?:^|\s)(@([^\s@]*))$`)
)

func ActiveAtToken(line string, cursorCol int) (Token, bool) {
	if cursorCol < 0 || cursorCol > len(line) {
		return Token{}, false
	}
	before := line[:cursorCol]
	if m := reQuoted.FindStringSubmatch(before); m != nil {
		return Token{Prefix: m[1], Query: m[2], Quoted: true}, true
	}
	if m := rePlain.FindStringSubmatch(before); m != nil {
		return Token{Prefix: m[1], Query: m[2], Quoted: false}, true
	}
	return Token{}, false
}

var unsafePath = regexp.MustCompile(`[\x00-\x1f\x7f-\x9f"]`)
var hasSpace = regexp.MustCompile(`\s`)

func FormatMention(path string, isDir, preserveQuote bool) string {
	if isDir && len(path) > 0 && path[len(path)-1] != '/' {
		path += "/"
	}
	if unsafePath.MatchString(path) {
		return ""
	}
	quoted := preserveQuote || hasSpace.MatchString(path)
	if !quoted {
		return "@" + path
	}
	if isDir {
		return `@"` + path
	}
	return `@"` + path + `"`
}
