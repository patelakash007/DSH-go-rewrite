//

//

package sysprompt

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
)

// ============================================================================

// ============================================================================

type bannedPattern struct {
	re   *regexp.Regexp
	note string
}

var bannedPatterns = []bannedPattern{
	{re: regexp.MustCompile(`time\.Now\(\)`), note: "time.Now() introduces dynamic time"},
	{re: regexp.MustCompile(`time\.Date\(`), note: "time.Date() fixed timestamp"},
	{re: regexp.MustCompile(`os\.Getwd\(\)`), note: "os.Getwd() relies on working directory"},
	{re: regexp.MustCompile(`math/rand`), note: "introduces random numbers"},
	{re: regexp.MustCompile(`\brand\.\w+\(`), note: "rand.*() random call"},
	{re: regexp.MustCompile(`os\.Getenv\("DSH_`), note: "dynamic DSH_* env access"},
	{re: regexp.MustCompile(`fmt\.Sprintf\(`), note: "fmt.Sprintf dynamic string format"},
}

func StaticCheck(text string) []string {
	var violations []string
	for _, bp := range bannedPatterns {
		if bp.re.MatchString(text) {
			violations = append(violations, bp.note)
		}
	}
	return violations
}

func CheckSections(a *Assembler) (violations []string, forbidden bool) {
	for _, s := range a.Sections() {
		for _, v := range StaticCheck(s.Text) {
			violations = append(violations, s.Name+": "+v)
		}
	}
	return violations, len(violations) > 0
}

// ============================================================================

// ============================================================================

func SystemHash(sys string) string {
	sum := sha256.Sum256([]byte(sys))
	return hex.EncodeToString(sum[:])
}

func (a *Assembler) AssembleHash() string {
	return SystemHash(a.Assemble())
}

func IsPure(sectionText string) bool {
	return len(StaticCheck(sectionText)) == 0
}

func StripBanned(s string) string {
	return strings.NewReplacer(
		"time.Now()", "NOW_STATIC",
		"os.Getwd()", "CWD_STATIC",
		"math/rand", "STATIC_RAND",
	).Replace(s)
}
