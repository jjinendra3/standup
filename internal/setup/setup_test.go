package setup

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strconv"
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
	if got := p.choose("pick", []string{"a", "b"}, 1); got != 2 {
		t.Errorf("out-of-range choice should be asked again, got %d", got)
	}
	// Input is exhausted: every question takes its default
	if got := p.ask("name", "fallback"); got != "fallback" {
		t.Errorf("got %q", got)
	}
}

func TestChooseSummarizerCustomCommand(t *testing.T) {
	// The last option is always the custom command, whatever agents are installed
	options := len(agents) + 1
	p := newPrompter(strconv.Itoa(options) + "\nllm -m gpt-5\n")
	s := config.Summarizer{Agent: config.AgentClaude, Model: "sonnet"}
	chooseSummarizer(p, &s)
	if s.Agent != config.AgentCommand || strings.Join(s.Command, " ") != "llm -m gpt-5" || s.Model != "" {
		t.Errorf("got %+v", s)
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
