//go:build unix

package run

import (
	"context"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestOutputKillsWhatTheCommandStartedWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	out, _ := Output(ctx, "", nil, "sh", "-c", "sleep 30 & echo $!; wait")
	pid, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil {
		t.Fatalf("no pid in output %q", out)
	}
	// The killed sleep disappears once its new parent reaps it
	deadline := time.Now().Add(5 * time.Second)
	for syscall.Kill(pid, 0) == nil {
		if time.Now().After(deadline) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
			t.Fatal("the background sleep outlived the cancelled command")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
