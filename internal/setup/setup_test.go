package setup

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jjinendra3/standup/internal/config"
)

func newPrompter(input string) prompter {
	return prompter{in: bufio.NewReader(strings.NewReader(input)), out: io.Discard}
}

func TestPrompterDefaultsAndRetries(t *testing.T) {
	p := newPrompter("\nmaybe\nn\n9\n2\n")
	if !p.confirm("first", true) {
		t.Error("empty answer should take the default")
	}
	if p.confirm("second", true) {
		t.Error("an invalid answer should be asked again, then n means no")
	}
	if got, err := p.choose("pick", []string{"a", "b"}, 1); got != 2 || err != nil {
		t.Errorf("out-of-range choice should be asked again, got %d, %v", got, err)
	}
	// Input is exhausted: every question takes its default
	if got := p.ask("name", "fallback"); got != "fallback" {
		t.Errorf("got %q", got)
	}
}

func TestChooseSummarizer(t *testing.T) {
	tests := []struct {
		name  string
		input string
		start config.Summarizer
		want  config.Summarizer
	}{
		{"claude with a listed model", "1\n2\n", config.Summarizer{}, config.Summarizer{Agent: config.AgentClaude, Model: "opus"}},
		{"claude with its own default", "1\n4\n", config.Summarizer{}, config.Summarizer{Agent: config.AgentClaude}},
		{"switching to claude suggests sonnet", "1\n\n", config.Summarizer{Agent: config.AgentCodex}, config.Summarizer{Agent: config.AgentClaude, Model: "sonnet"}},
		{"a model set by hand stays the suggestion", "1\n\n", config.Summarizer{Agent: config.AgentClaude, Model: "claude-opus-5-5"}, config.Summarizer{Agent: config.AgentClaude, Model: "claude-opus-5-5"}},
		{"codex is not asked for a model", "2\n", config.Summarizer{Agent: config.AgentClaude, Model: "opus"}, config.Summarizer{Agent: config.AgentCodex}},
		{"codex keeps a model set by hand", "2\n", config.Summarizer{Agent: config.AgentCodex, Model: "gpt-5"}, config.Summarizer{Agent: config.AgentCodex, Model: "gpt-5"}},
		{"a custom command is replaced", "1\n\n", config.Summarizer{Agent: config.AgentCommand, Command: []string{"llm"}}, config.Summarizer{Agent: config.AgentClaude, Model: "sonnet"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.start
			if err := chooseSummarizer(newPrompter(tt.input), &s); err != nil {
				t.Fatal(err)
			}
			if s.Agent != tt.want.Agent || s.Model != tt.want.Model || s.Command != nil {
				t.Errorf("got %+v, want %+v", s, tt.want)
			}
		})
	}
}

func TestChooseSummarizerOffersOnlyClaudeAndCodex(t *testing.T) {
	var out strings.Builder
	p := prompter{in: bufio.NewReader(strings.NewReader("")), out: &out}
	if err := chooseSummarizer(p, &config.Summarizer{}); err != nil {
		t.Fatal(err)
	}
	agentQuestion, _, _ := strings.Cut(out.String(), "Model to use")
	if strings.Contains(agentQuestion, "3)") || !strings.Contains(agentQuestion, "1) Claude Code") || !strings.Contains(agentQuestion, "2) Codex") {
		t.Errorf("agent options:\n%s", agentQuestion)
	}
}

func TestChooseRepoDirsAcceptsTypedFolders(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "api", ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{RepoDepth: 2}
	chooseRepoDirs(newPrompter(root+"\n\n"), &cfg)
	if len(cfg.RepoDirs) != 1 || cfg.RepoDirs[0] != root {
		t.Errorf("got %v, want [%s]", cfg.RepoDirs, root)
	}
}
