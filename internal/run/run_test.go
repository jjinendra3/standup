package run

import (
	"context"
	"os"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestOutputReturnsSoonAfterCancelWhileAChildHoldsItsOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	// Cancelling kills sh, but the sleep it started in the background keeps stdout and stderr open
	out, err := Output(ctx, "", nil, "sh", "-c", "sleep 30 & echo $!; wait")
	elapsed := time.Since(start)
	if pid, convErr := strconv.Atoi(strings.TrimSpace(out)); convErr == nil {
		if p, findErr := os.FindProcess(pid); findErr == nil {
			_ = p.Kill()
		}
	}
	if err == nil {
		t.Error("want an error from the cancelled command")
	}
	if elapsed > 10*time.Second {
		t.Errorf("Output returned %v after the command started; it waited for the child holding its output", elapsed)
	}
}

func TestOutputSucceedsWhenTheCommandLeavesAChildHoldingItsOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	start := time.Now()
	// Like xclip, sh exits at once and leaves a background process with stdout and stderr open
	out, err := Output(context.Background(), "", nil, "sh", "-c", "sleep 30 & echo $!; echo done")
	elapsed := time.Since(start)
	pid, _, _ := strings.Cut(out, "\n")
	if n, convErr := strconv.Atoi(pid); convErr == nil {
		if p, findErr := os.FindProcess(n); findErr == nil {
			_ = p.Kill()
		}
	}
	if err != nil || !strings.HasSuffix(out, "done\n") {
		t.Errorf("Output = %q, %v; want the output and no error", out, err)
	}
	if elapsed > 10*time.Second {
		t.Errorf("Output returned %v after the command started; it waited for the child holding its output", elapsed)
	}
}
