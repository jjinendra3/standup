package setup

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jjinendra3/standup/internal/config"
)

func newPrompter(input string) prompter {
	return prompter{ctx: context.Background(), in: bufio.NewReader(strings.NewReader(input)), out: io.Discard}
}

func TestPrompterDefaultsAndRetries(t *testing.T) {
	p := newPrompter("\nmaybe\nn\n9\n2\n")
	if yes, err := p.confirm("first", true); !yes || err != nil {
		t.Errorf("empty answer should take the default, got %v, %v", yes, err)
	}
	if yes, err := p.confirm("second", true); yes || err != nil {
		t.Errorf("an invalid answer should be asked again, then n means no, got %v, %v", yes, err)
	}
	if got, err := p.choose("pick", []string{"a", "b"}, 1); got != 2 || err != nil {
		t.Errorf("out-of-range choice should be asked again, got %d, %v", got, err)
	}
	// Input is exhausted: every question takes its default
	if got, err := p.ask("name", "fallback"); got != "fallback" || err != nil {
		t.Errorf("got %q, %v", got, err)
	}
}

func TestPrompterStopsWhenCancelled(t *testing.T) {
	// A pipe nobody writes to blocks like a terminal waiting for an answer
	r, w := io.Pipe()
	defer w.Close()
	ctx, cancel := context.WithCancel(context.Background())
	var out strings.Builder
	p := prompter{ctx: ctx, in: bufio.NewReader(r), out: &out}

	done := make(chan error, 1)
	go func() {
		_, err := p.confirm("Use these folders?", true)
		done <- err
	}()
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("confirm returned %v, want context.Canceled", err)
	}
	if !strings.HasSuffix(out.String(), "[Y/n]: \n") {
		t.Errorf("output %q does not end the prompt line", out.String())
	}
	if _, err := p.ask("Folders", "~/code"); !errors.Is(err, context.Canceled) {
		t.Errorf("ask after the cancel returned %v", err)
	}
	if _, err := p.choose("pick", []string{"a", "b"}, 1); !errors.Is(err, context.Canceled) {
		t.Errorf("choose after the cancel returned %v", err)
	}
}

func TestChooseSourcesStopsWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r, w := io.Pipe()
	defer w.Close()
	cfg := config.Config{Sources: []string{"git"}}
	p := prompter{ctx: ctx, in: bufio.NewReader(r), out: io.Discard}
	if err := chooseSources(p, &cfg, true, false); !errors.Is(err, context.Canceled) {
		t.Errorf("chooseSources returned %v, want context.Canceled", err)
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
	p := prompter{ctx: context.Background(), in: bufio.NewReader(strings.NewReader("")), out: &out}
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
	if err := chooseRepoDirs(newPrompter(root+"\n\n"), &cfg); err != nil {
		t.Fatal(err)
	}
	if len(cfg.RepoDirs) != 1 || cfg.RepoDirs[0] != root {
		t.Errorf("got %v, want [%s]", cfg.RepoDirs, root)
	}
}
