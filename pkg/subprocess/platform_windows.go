//go:build windows

package subprocess

import (
	"os/exec"
	"syscall"
)

const (
	syscallSIGTERM = 0
	syscallSIGKILL = 0
)

func platformSetSysProcAttr(cmd *exec.Cmd) {
	if cmd.SysProcAttr != nil {
		cmd.SysProcAttr.CreationFlags |= syscall.CREATE_NEW_PROCESS_GROUP
		return
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}

func platformExitSignal(*exec.ExitError) string { return "" }

func platformSignalGroup(int, int) error { return nil }
