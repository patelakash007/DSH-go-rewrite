//

//

package sandbox

import (
	"os"
	"path/filepath"
)

//

func CanonicalPath(path string) string {
	if path == "" {
		return path
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {

		return filepath.Clean(path)
	}
	abs, err := filepath.Abs(resolved)
	if err != nil {
		return filepath.Clean(resolved)
	}
	return filepath.Clean(abs)
}

//

func WritableRoots(policy SandboxExecutionPolicy) []string {
	if policy.Mode != ModeWorkspaceWrite {
		return nil
	}
	roots := []string{
		policy.WorkspaceRoot,
		"/tmp",
		os.TempDir(),
	}
	seen := map[string]bool{}
	var result []string
	for _, r := range roots {
		if r == "" {
			continue
		}
		canonical := CanonicalPath(r)
		if seen[canonical] {
			continue
		}
		seen[canonical] = true
		result = append(result, canonical)
	}
	return result
}
