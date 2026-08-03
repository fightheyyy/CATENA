//go:build windows

package control

import (
	"os"
	"os/exec"
)

func prepareCommand(_ *exec.Cmd) {}

func interruptCommand(cmd *exec.Cmd) error {
	return cmd.Process.Signal(os.Interrupt)
}

func killCommand(cmd *exec.Cmd) error {
	return cmd.Process.Kill()
}
