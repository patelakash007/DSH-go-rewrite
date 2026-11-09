//

//

//

package lint

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

type Violation struct {
	Path string

	Line int

	Pattern string
}

func (v Violation) Error() string {
	return fmt.Sprintf("%s:%d: %s", v.Path, v.Line, v.Pattern)
}

func ScanDir(dir string) ([]Violation, error) {
	var out []Violation
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		vs, err := ScanFile(path)
		if err != nil {
			return nil
		}
		out = append(out, vs...)
		return nil
	})
	if err != nil {
		return out, err
	}
	return out, nil
}

func ScanFile(path string) ([]Violation, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.AllErrors)
	if err != nil {
		return nil, err
	}
	imports := map[string]bool{}
	for _, imp := range f.Imports {
		imports[strings.Trim(imp.Path.Value, `"`)] = true
	}
	var out []Violation
	ast.Inspect(f, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		line := fset.Position(sel.Pos()).Line
		name := sel.Sel.Name
		switch {
		case sel.X != nil && exprName(sel.X) == "time" && (name == "Now" || name == "Date"):
			out = append(out, Violation{Path: path, Line: line, Pattern: "dynamic time time." + name})
		case sel.X != nil && exprName(sel.X) == "os" && name == "Getwd":
			out = append(out, Violation{Path: path, Line: line, Pattern: "relies on working directory os.Getwd"})
		case sel.X != nil && exprName(sel.X) == "rand" && isImport(imports, "math/rand"):
			out = append(out, Violation{Path: path, Line: line, Pattern: "randomness rand." + name})
		case sel.X != nil && exprName(sel.X) == "math" && name == "Rand":
			_ = name
		}
		return true
	})

	if strings.Contains(readSource(path), "json.Marshal(map") {
		out = append(out, Violation{Path: path, Line: 0, Pattern: "unordered map serialization json.Marshal(map)"})
	}
	return out, nil
}

func exprName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		return exprName(t.X) + "." + t.Sel.Name
	}
	return ""
}

func isImport(imports map[string]bool, pkg string) bool {
	return imports[pkg]
}

func readSource(path string) string {
	b, _ := os.ReadFile(path)
	return string(b)
}
