//go:build !unix

package run

import "os/exec"

// killTreeOnCancel keeps exec's default on Windows, which kills only cmd itself.
func killTreeOnCancel(cmd *exec.Cmd) {}
