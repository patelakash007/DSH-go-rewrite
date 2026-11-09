//

//

package sandbox

import (
	"os/exec"
	"runtime"
	"sync"
)

type BackendType string

const (
	BackendBwrap BackendType = "bwrap"

	BackendLandlock BackendType = "landlock"

	BackendSeatbelt BackendType = "seatbelt"

	BackendWindowsACL BackendType = "windows-acl"

	BackendUnavailable BackendType = "unavailable"
)

type LocalConfig struct {
	RunnerCommand string

	BackendType BackendType
}

type LocalProvider struct {
	config  LocalConfig
	once    sync.Once
	backend BackendType
	runner  string
}

func NewLocalProvider(config *LocalConfig) *LocalProvider {
	p := &LocalProvider{}
	if config != nil {
		p.config = *config
	}
	return p
}

func (p *LocalProvider) detectBackend() (BackendType, string) {
	p.once.Do(func() {

		if p.config.RunnerCommand != "" && p.config.BackendType != "" {
			p.backend = p.config.BackendType
			p.runner = p.config.RunnerCommand
			return
		}

		switch runtime.GOOS {
		case "linux":

			if p.tryBwrap() {
				p.backend = BackendBwrap
				p.runner = "bwrap"
				return
			}
			if p.tryLandlock() {
				p.backend = BackendLandlock
				p.runner = "landlock-launcher"
				return
			}
		case "darwin":

			if p.trySeatbelt() {
				p.backend = BackendSeatbelt
				p.runner = "sandbox-exec"
				return
			}
		case "windows":

			p.backend = BackendWindowsACL
			p.runner = "windows-acl"
			return
		}
		p.backend = BackendUnavailable
		p.runner = ""
	})
	return p.backend, p.runner
}

func (p *LocalProvider) tryBwrap() bool {
	_, err := exec.LookPath("bwrap")
	if err != nil {
		return false
	}

	cmd := exec.Command("bwrap",
		"--ro-bind", "/", "/",
		"--dev", "/dev",
		"--unshare-pid",
		"--proc", "/proc",
		"--die-with-parent",
		"--", "true",
	)
	return cmd.Run() == nil
}

func (p *LocalProvider) tryLandlock() bool {
	_, err := exec.LookPath("landlock-launcher")
	return err == nil
}

func (p *LocalProvider) trySeatbelt() bool {
	_, err := exec.LookPath("sandbox-exec")
	if err != nil {
		return false
	}

	cmd := exec.Command("sandbox-exec", "-p",
		"(version 1)(allow default)(deny file-write*)(allow file-write* (literal \"/dev/null\"))",
		"true",
	)
	return cmd.Run() == nil
}

func (p *LocalProvider) Backend() BackendType {
	b, _ := p.detectBackend()
	return b
}

func (p *LocalProvider) Runner() string {
	_, r := p.detectBackend()
	return r
}

func (p *LocalProvider) Available() bool {
	b, _ := p.detectBackend()
	return b != BackendUnavailable
}

func (p *LocalProvider) Confine(argv []string, policy SandboxPolicy) (ConfinedArgv, error) {
	backend, runner := p.detectBackend()
	if backend == BackendUnavailable {
		return ConfinedArgv{}, &SandboxUnavailableError{Msg: "no local sandbox backend available on this platform"}
	}

	if backend == BackendWindowsACL {

		return ConfinedArgv{
			Argv:             argv,
			Enforcement:      EnforcementFull,
			DenialSignatures: p.DenialSignatures(),
		}, nil
	}

	var profileArgs []string
	switch backend {
	case BackendBwrap:
		profileArgs = BwrapProfileArgs(policy)
	case BackendLandlock:
		profileArgs = LandlockProfileArgs(policy)
	case BackendSeatbelt:
		profileArgs = SeatbeltProfileArgs(policy)
	}

	fullArgv := []string{runner}
	fullArgv = append(fullArgv, profileArgs...)
	fullArgv = append(fullArgv, "--")
	fullArgv = append(fullArgv, argv...)
	return ConfinedArgv{
		Argv:             fullArgv,
		Enforcement:      EnforcementFull,
		DenialSignatures: p.DenialSignatures(),
	}, nil
}

func (p *LocalProvider) DenialSignatures() []string {
	backend, _ := p.detectBackend()
	switch backend {
	case BackendBwrap:
		return []string{"Read-only file system", "EROFS"}
	case BackendLandlock:
		return []string{"Permission denied", "EACCES"}
	case BackendSeatbelt:
		return []string{"Operation not permitted", "EPERM"}
	case BackendWindowsACL:
		return []string{"Access is denied", "ERROR_ACCESS_DENIED"}
	default:
		return nil
	}
}
