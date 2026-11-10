//go:build !linux && !android

package main

import (
	"errors"
	"os"
)

func openPTY() (*os.File, *os.File, error) {
	return nil, nil, errors.New("PTY allocation is not supported on this platform")
}

func setPTYSize(file *os.File, cols, rows int) error {
	return errors.New("PTY resize is not supported on this platform")
}

func setPTYSizeFD(fd int, cols, rows int) error {
	return errors.New("PTY resize is not supported on this platform")
}
