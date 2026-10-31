// Package run executes external commands and reports their stderr on failure.
package run

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
)

// waitDelay caps how long Output waits for the command's output pipes to close after it exits or ctx is done.
// Without it, a child process the command started, such as an agent's helper, keeps them open and Ctrl-C hangs.
const waitDelay = time.Second

// Output runs name with args in dir (the current directory when empty), feeding stdin when it is not nil,
// and returns stdout. On failure the error includes the last line of stderr.
func Output(ctx context.Context, dir string, stdin io.Reader, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Stdin = stdin
	cmd.WaitDelay = waitDelay
	killTreeOnCancel(cmd)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	// ErrWaitDelay means the command succeeded but left a process holding its output, as xclip does while it serves the clipboard
	if err := cmd.Run(); err != nil && !errors.Is(err, exec.ErrWaitDelay) {
		if msg := LastLine(stderr.String()); msg != "" {
			return stdout.String(), fmt.Errorf("%s: %w: %s", name, err, msg)
		}
		return stdout.String(), fmt.Errorf("%s: %w", name, err)
	}
	return stdout.String(), nil
}

// LastLine returns the last non-empty line of s.
func LastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}
