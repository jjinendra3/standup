//go:build unix

package run

import (
	"os/exec"
	"syscall"
)

// killTreeOnCancel starts cmd in its own session so cancelling kills everything it started, such as an agent's
// helpers, not just cmd. With no terminal, a child that tries to read one fails instead of stopping.
func killTreeOnCancel(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
}
