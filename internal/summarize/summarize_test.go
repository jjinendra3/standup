package summarize

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/jjinendra3/standup/internal/config"
	"github.com/jjinendra3/standup/internal/window"
)

func TestPromptUsesWindowHeadingsAndRules(t *testing.T) {
	w := window.For(window.Yesterday, time.Date(2026, 10, 21, 9, 0, 0, 0, time.Local))
	p := Prompt(w, []string{"Write in British English", "- Mention ticket IDs"}, "## Git\n- did a thing\n")
	for _, want := range []string{
		"Yesterday:\n- one bullet per work item\nToday:\n",
		"Include PR reviews as bullets under Yesterday.",
		"\n- Write in British English\n- Mention ticket IDs\n",
		"---\n\n## Git\n- did a thing\n",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt is missing %q\n%s", want, p)
		}
	}
}

// TestAgentEndToEnd runs a real agent CLI with a dummy prompt. It is opt-in because it needs the agent
// installed and logged in: STANDUP_E2E_AGENT=codex [STANDUP_E2E_MODEL=…] go test ./internal/summarize -run EndToEnd
func TestAgentEndToEnd(t *testing.T) {
	agent := os.Getenv("STANDUP_E2E_AGENT")
	if agent == "" {
		t.Skip("set STANDUP_E2E_AGENT to claude or codex to run")
	}
	out, err := Run(context.Background(), config.Summarizer{Agent: agent, Model: os.Getenv("STANDUP_E2E_MODEL")}, "Reply with exactly the word: pong")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s replied: %q", agent, out)
	if !strings.Contains(strings.ToLower(out), "pong") {
		t.Errorf("unexpected reply %q", out)
	}
}

func TestCustomCommandGetsPromptOnStdin(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	// git hash-object reads stdin and prints a hash, so any portable stdin reader proves the wiring
	out, err := Run(context.Background(), config.Summarizer{Agent: config.AgentCommand, Command: []string{"git", "hash-object", "--stdin"}}, "hello\n")
	if err != nil {
		t.Fatal(err)
	}
	if out != "ce013625030ba8dba906f756967f9e9ca394464a" {
		t.Errorf("got %q", out)
	}
}
