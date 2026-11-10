//go:build linux || android

package main

import (
	"fmt"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

// openPTY allocates a pseudo-terminal pair through /dev/ptmx. It avoids CGO so
// the Android engine keeps building with CGO_ENABLED=0.
func openPTY() (*os.File, *os.File, error) {
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		return nil, nil, err
	}
	if err := unix.IoctlSetPointerInt(int(master.Fd()), unix.TIOCSPTLCK, 0); err != nil {
		_ = master.Close()
		return nil, nil, err
	}
	number, err := unix.IoctlGetInt(int(master.Fd()), unix.TIOCGPTN)
	if err != nil {
		_ = master.Close()
		return nil, nil, err
	}
	slave, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", number), os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		_ = master.Close()
		return nil, nil, err
	}
	return master, slave, nil
}

func setPTYSize(file *os.File, cols, rows int) error {
	if file == nil {
		return fmt.Errorf("terminal is not a PTY")
	}
	return unix.IoctlSetWinsize(int(file.Fd()), unix.TIOCSWINSZ, &unix.Winsize{
		Col: uint16(cols), Row: uint16(rows),
	})
}
