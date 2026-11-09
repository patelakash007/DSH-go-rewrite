//

//

//     (version 1)(allow default)(deny file-write*)(allow file-write* (literal /dev/null))，

package sandbox

import (
	"fmt"
	"strings"
)

//

//

func BwrapProfileArgs(policy SandboxPolicy) []string {
	args := []string{
		"--ro-bind", "/", "/",
		"--dev", "/dev",
		"--unshare-pid",
		"--proc", "/proc",
		"--die-with-parent",
	}
	if policy.Mode == ConfinedWorkspaceWrite {
		args = append(args, "--tmpfs", "/tmp")
		args = append(args, "--bind", policy.WorkspaceRoot, policy.WorkspaceRoot)
	}
	return args
}

//

//   - readOnly: [/]

//

// - readOnly: [/]
// - readWrite: [/dev/null, /tmp, workspaceRoot]
func LandlockProfileArgs(policy SandboxPolicy) []string {
	readWrite := []string{"/dev/null"}
	if policy.Mode == ConfinedWorkspaceWrite {
		readWrite = append(readWrite, "/tmp", policy.WorkspaceRoot)
	}
	args := []string{}
	// readOnly
	args = append(args, "--read-only", "/")
	// readWrite
	for _, rw := range readWrite {
		args = append(args, "--read-write", rw)
	}
	return args
}

//

//   - (version 1)

//

func SeatbeltProfileArgs(policy SandboxPolicy) []string {
	forms := []string{
		"(version 1)",
		"(allow default)",
		"(deny file-write*)",
		fmt.Sprintf("(allow file-write* (literal %s))", sbplString("/dev/null")),
	}
	if policy.Mode == ConfinedWorkspaceWrite {
		roots := WritableRoots(policy.SandboxExecutionPolicy)
		if len(roots) > 0 {
			var subpaths []string
			for _, root := range roots {
				subpaths = append(subpaths, fmt.Sprintf("(subpath %s)", sbplString(root)))
			}
			forms = append(forms, fmt.Sprintf("(allow file-write* %s)", strings.Join(subpaths, " ")))
		}
	}
	return []string{"-p", strings.Join(forms, " ")}
}

func sbplString(path string) string {
	escaped := strings.ReplaceAll(path, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return `"` + escaped + `"`
}
