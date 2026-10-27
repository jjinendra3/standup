// Package summarize turns an activity log into a standup by piping it to a coding agent's CLI.
package summarize

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jjinendra3/standup/internal/config"
	"github.com/jjinendra3/standup/internal/run"
	"github.com/jjinendra3/standup/internal/window"
)

// promptTemplate holds the instructions; %[1]s and %[2]s are the window's done and next headings.
//
//go:embed prompt.txt
var promptTemplate string

// Prompt is the full instruction and activity log sent to the agent.
func Prompt(w window.Window, rules []string, activityLog string) string {
	var b strings.Builder
	fmt.Fprintf(&b, promptTemplate, w.DoneHeading, w.NextHeading)
	for _, r := range rules {
		b.WriteString("- " + strings.TrimPrefix(r, "- ") + "\n")
	}
	b.WriteString("\n---\n\n" + activityLog)
	return b.String()
}

// Agents that standup knows how to run, in the order it prefers them when none is configured.
var Agents = []string{config.AgentClaude, config.AgentCodex}

// ResolveAgent returns the configured agent, or the first known agent on PATH when none is set.
func ResolveAgent(s config.Summarizer) (string, error) {
	if s.Agent != "" {
		return s.Agent, nil
	}
	for _, a := range Agents {
		if _, err := exec.LookPath(a); err == nil {
			return a, nil
		}
	}
	return "", errors.New("no coding agent found to write the standup. Install Claude Code or Codex, or run standup setup to use another command")
}

// Run sends prompt to the configured agent and returns the standup it writes.
// Agents run without tools or write access, in an empty folder so no project instructions get loaded.
func Run(ctx context.Context, s config.Summarizer, prompt string) (string, error) {
	agent, err := ResolveAgent(s)
	if err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp("", "standup-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)

	var out string
	switch agent {
	case config.AgentClaude:
		args := []string{"-p", "--tools", "", "--no-session-persistence"}
		if s.Model != "" {
			args = append(args, "--model", s.Model)
		}
		out, err = runAgent(ctx, dir, prompt, "claude", args...)
	case config.AgentCodex:
		// Read the final message from the file codex writes rather than parsing its console output
		last := filepath.Join(dir, "last-message.txt")
		args := []string{"exec", "--sandbox", "read-only", "--skip-git-repo-check", "--ephemeral", "--color", "never", "-o", last}
		if s.Model != "" {
			args = append(args, "-m", s.Model)
		}
		if _, err = runAgent(ctx, dir, prompt, "codex", args...); err == nil {
			var b []byte
			b, err = os.ReadFile(last)
			out = string(b)
		}
	case config.AgentCommand:
		if len(s.Command) == 0 {
			return "", errors.New(`summarizer agent is "command" but no command is set in the config`)
		}
		out, err = runAgent(ctx, dir, prompt, s.Command[0], s.Command[1:]...)
	default:
		return "", fmt.Errorf("unknown summarizer agent %q. Use claude, codex or command", agent)
	}
	if err != nil {
		return "", err
	}
	if out = strings.TrimSpace(out); out == "" {
		return "", fmt.Errorf("%s returned an empty standup", agent)
	}
	return out, nil
}

func runAgent(ctx context.Context, dir, prompt, name string, args ...string) (string, error) {
	if _, err := exec.LookPath(name); err != nil {
		return "", fmt.Errorf("%s is not installed or not on PATH. Run standup setup to pick another agent", name)
	}
	return run.Output(ctx, dir, strings.NewReader(prompt), name, args...)
}
