//

package tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/JopenChen/dsh-go/internal/lint"
	"github.com/JopenChen/dsh-go/pkg/tools"
)

const badSource = `package p
import "time"
import "os"
var x = time.Now()
var y = os.Getwd()
`

const badRand = `package p
import "math/rand"
var r = rand.Intn(10)
`

const cleanSource = `package p
func f() string { return "static" }
`

func TestN06LintCatchesDynamicPatterns(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte(badSource), 0o644); err != nil {
		t.Fatal(err)
	}
	vs, err := lint.ScanDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) < 2 {
		t.Fatalf("should catch time.Now and os.Getwd, got %d matches: %v", len(vs), vs)
	}
}

func TestN06LintCatchesRand(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "r.go"), []byte(badRand), 0o644); err != nil {
		t.Fatal(err)
	}
	vs, _ := lint.ScanDir(dir)
	if len(vs) == 0 {
		t.Fatal("should catch math/rand randomness anti-pattern")
	}
}

func TestN06LintCleanPackage(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "clean.go"), []byte(cleanSource), 0o644); err != nil {
		t.Fatal(err)
	}
	vs, _ := lint.ScanDir(dir)
	if len(vs) != 0 {
		t.Fatalf("clean package should have no violations, got %v", vs)
	}
}

func TestN06SchemaDeterministicOrder(t *testing.T) {

	schema := &tools.JsonSchemaNode{
		Type: tools.TypeObject,
		Properties: map[string]*tools.JsonSchemaNode{
			"b": {Type: tools.TypeString},
			"a": {Type: tools.TypeInteger},
			"c": {Type: tools.TypeBoolean},
		},
		Required: []string{"c", "a"},
	}
	b1, err := schema.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 1000; i++ {
		b2, _ := schema.Marshal()
		if string(b1) != string(b2) {
			t.Fatalf("marshal mismatch on attempt %d (random schema order)", i)
		}
	}

	s := string(b1)
	if posOfStr(s, "\"a\"") > posOfStr(s, "\"c\"") {
		t.Fatalf("schema should be lexicographical: %s", s)
	}
}

func posOfStr(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
