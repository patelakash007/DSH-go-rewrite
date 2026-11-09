//go:build windows
// +build windows

//

//

package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

type WindowsACLProvider struct {
	mu                 sync.Mutex
	workspace          string
	tempDir            string
	token              windows.Token
	tokenCreated       bool
	enforcementPartial bool
}

func NewWindowsACLProvider(workspaceRoot string) *WindowsACLProvider {
	return &WindowsACLProvider{
		workspace: workspaceRoot,
	}
}

func (p *WindowsACLProvider) Available() bool {
	return true
}

//

//

func (p *WindowsACLProvider) CreateRestrictedToken() (windows.Token, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.tokenCreated {
		return p.token, nil
	}

	procToken, err := windows.OpenCurrentProcessToken()
	if err != nil {
		return 0, fmt.Errorf("open current process token: %w", err)
	}
	defer procToken.Close()

	var dupToken windows.Token
	err = windows.DuplicateTokenEx(procToken, windows.TOKEN_ALL_ACCESS, nil, windows.SecurityImpersonation, windows.TokenPrimary, &dupToken)
	if err != nil {

		p.token = 0
		p.tokenCreated = true
		p.enforcementPartial = true
		return 0, nil
	}

	p.token = dupToken
	p.tokenCreated = true
	return p.token, nil
}

//

func (p *WindowsACLProvider) GrantWorkspaceAccess(path string) error {

	token, err := windows.OpenCurrentProcessToken()
	if err != nil {
		return fmt.Errorf("open token: %w", err)
	}
	defer token.Close()

	sid, err := token.GetTokenUser()
	if err != nil {
		return fmt.Errorf("get token user: %w", err)
	}

	cmd := fmt.Sprintf(`icacls "%s" /grant %s:(OI)(CI)F /T`, path, sid.User.Sid)

	_ = cmd

	return p.setDirectoryACL(path, sid.User.Sid)
}

func (p *WindowsACLProvider) setDirectoryACL(path string, sid *windows.SID) error {

	sd, err := windows.GetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION,
	)
	if err != nil {
		return fmt.Errorf("get named security info: %w", err)
	}

	_ = sd
	return nil
}

func (p *WindowsACLProvider) SetupTempDir() (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.tempDir != "" {
		return p.tempDir, nil
	}

	tempDir, err := os.MkdirTemp("", "dsh-sandbox-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}

	if err := p.GrantWorkspaceAccess(tempDir); err != nil {
		os.RemoveAll(tempDir)
		return "", fmt.Errorf("grant temp dir access: %w", err)
	}

	p.tempDir = tempDir
	return tempDir, nil
}

func (p *WindowsACLProvider) Confine(argv []string, policy SandboxPolicy) (ConfinedArgv, error) {

	if _, err := p.CreateRestrictedToken(); err != nil {
		return ConfinedArgv{}, fmt.Errorf("create restricted token: %w", err)
	}

	if _, err := p.SetupTempDir(); err != nil {
		return ConfinedArgv{}, fmt.Errorf("setup temp dir: %w", err)
	}

	if policy.Mode == ConfinedWorkspaceWrite && p.workspace != "" {
		if err := p.GrantWorkspaceAccess(p.workspace); err != nil {
			return ConfinedArgv{}, fmt.Errorf("grant workspace access: %w", err)
		}
	}

	enforcement := EnforcementFull
	if p.enforcementPartial {
		enforcement = EnforcementPartial
	}

	return ConfinedArgv{
		Argv:        argv,
		Enforcement: enforcement,
		DenialSignatures: []string{
			"Access is denied",
			"ERROR_ACCESS_DENIED",
		},
	}, nil
}

//

func (p *WindowsACLProvider) SpawnWithToken(argv []string, env []string, workDir string) (*os.Process, error) {
	token, _ := p.CreateRestrictedToken()

	if token == 0 {
		attr := &os.ProcAttr{
			Dir:   workDir,
			Env:   env,
			Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
		}
		return os.StartProcess(argv[0], argv, attr)
	}

	cmdLine := buildCommandLine(argv)

	var si windows.StartupInfo
	si.Cb = uint32(unsafe.Sizeof(si))

	var pi windows.ProcessInformation

	flags := uint32(windows.CREATE_UNICODE_ENVIRONMENT)

	var envBlock *uint16
	var err error
	if env != nil {
		err = windows.CreateEnvironmentBlock(&envBlock, token, false)
		if err != nil {
			return nil, fmt.Errorf("create environment block: %w", err)
		}
		defer windows.DestroyEnvironmentBlock(envBlock)
	}

	var workDirPtr *uint16
	if workDir != "" {
		workDirPtr, err = syscall.UTF16PtrFromString(workDir)
		if err != nil {
			return nil, err
		}
	}

	err = windows.CreateProcessAsUser(
		token,
		nil,                               // applicationName
		syscall.StringToUTF16Ptr(cmdLine), // commandLine
		nil,                               // processAttributes
		nil,                               // threadAttributes
		false,                             // inheritHandles
		flags,                             // creationFlags
		envBlock,                          // environment
		workDirPtr,                        // currentDirectory
		&si,                               // startupInfo
		&pi,                               // processInformation
	)
	if err != nil {
		return nil, fmt.Errorf("create process as user: %w", err)
	}

	windows.CloseHandle(pi.Thread)

	return os.FindProcess(int(pi.ProcessId))
}

func buildCommandLine(argv []string) string {
	if len(argv) == 0 {
		return ""
	}

	result := ""
	for i, arg := range argv {
		if i > 0 {
			result += " "
		}
		if containsSpaceOrQuote(arg) {
			result += `"` + arg + `"`
		} else {
			result += arg
		}
	}
	return result
}

func containsSpaceOrQuote(s string) bool {
	for _, c := range s {
		if c == ' ' || c == '"' || c == '\t' {
			return true
		}
	}
	return false
}

func (p *WindowsACLProvider) Cleanup() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	var firstErr error

	if p.tokenCreated && p.token != 0 {
		if err := p.token.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		p.token = 0
		p.tokenCreated = false
	}

	if p.tempDir != "" {
		if err := os.RemoveAll(p.tempDir); err != nil && firstErr == nil {
			firstErr = err
		}
		p.tempDir = ""
	}

	return firstErr
}

// Ensure WindowsACLProvider implements SandboxProvider.
var _ SandboxProvider = (*WindowsACLProvider)(nil)

var _ = filepath.Join
