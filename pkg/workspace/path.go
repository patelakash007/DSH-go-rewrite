package workspace

import (
	"path/filepath"
	"strings"
)

func ResolvePath(root, path string) string {
	if isAbsolutePath(path) {
		return path
	}
	if root == "" {
		return path
	}
	return filepath.Join(root, path)
}

func isAbsolutePath(p string) bool {
	if filepath.IsAbs(p) {
		return true
	}

	if strings.HasPrefix(p, "/") || strings.HasPrefix(p, `\`) {
		return true
	}
	return false
}

func AbbreviateHome(path, home string) string {
	if home == "" {
		return path
	}
	root := strings.TrimRight(home, `/\`)
	if root == "" {
		return path
	}
	if path == root {
		return "~"
	}

	if strings.HasPrefix(path, root+"/") || strings.HasPrefix(path, root+`\`) {
		return "~" + strings.TrimPrefix(path, root)
	}
	return path
}

func TitleOf(path string) string {
	return filepath.Base(strings.TrimRight(path, `/\`))
}
