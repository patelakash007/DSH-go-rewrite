//go:build !windows

package subprocess

import (
	"os/exec"
	"syscall"
)

const (
	syscallSIGTERM = int(syscall.SIGTERM)
	syscallSIGKILL = int(syscall.SIGKILL)
)

func platformSetSysProcAttr(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

func platformExitSignal(ee *exec.ExitError) string {
	if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		return ws.Signal().String()
	}
	return ""
}

func platformSignalGroup(pid, sig int) error {
	_ = sig
	return syscall.Kill(pid, syscall.Signal(sig))
}
