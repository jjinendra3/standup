// Package setup runs the interactive configuration: tools, summarizer, sources and repo folders.
package setup

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/jjinendra3/standup/internal/activity"
	"github.com/jjinendra3/standup/internal/config"
	"github.com/jjinendra3/standup/internal/picker"
	"github.com/jjinendra3/standup/internal/run"
	"github.com/jjinendra3/standup/internal/summarize"
)

type agentInfo struct {
	label        string
	defaultModel string
	// models are offered in setup; with none, setup does not ask and the agent uses its own default
	models  []string
	install string
}

var agents = map[string]agentInfo{
	config.AgentClaude: {"Claude Code (claude)", "sonnet", []string{"sonnet", "opus", "fable"}, "Install Claude Code from https://claude.com/claude-code"},
	config.AgentCodex:  {"Codex (codex)", "", nil, "Install Codex with: npm install -g @openai/codex"},
}

// Run asks the setup questions on in and out, then saves the settings file.
func Run(ctx context.Context, in io.Reader, out io.Writer) error {
	p := prompter{in: bufio.NewReader(in), out: out}
	if tty, ok := in.(*os.File); ok {
		if o, ok := out.(*os.File); ok && picker.Available(tty, o) {
			p.tty = tty
		}
	}
	cfg, _, err := config.Load()
	if err != nil {
		return err
	}
	fmt.Fprintln(out, "Setting up standup. Press Enter to accept the answer in [brackets].")

	p.section("1. Tools")
	gitOK := checkGit(p)
	ghOK := ensureGH(ctx, p)

	p.section("2. Who writes the standup")
	if err := chooseSummarizer(p, &cfg.Summarizer); err != nil {
		return err
	}

	p.section("3. Activity sources")
	chooseSources(p, &cfg, gitOK, ghOK)

	if cfg.HasSource("git") {
		p.section("4. Repo folders")
		chooseRepoDirs(p, &cfg)
	}

	path, err := config.Save(cfg)
	if err != nil {
		return fmt.Errorf("saving settings: %w", err)
	}
	fmt.Fprintf(out, "\nSaved %s\nYou're set. Run: standup today   (or: standup yesterday)\n", path)
	return nil
}

func checkGit(p prompter) bool {
	if _, err := exec.LookPath("git"); err == nil {
		p.ok("git is installed")
		return true
	}
	hint := "install git with your package manager"
	switch runtime.GOOS {
	case "darwin":
		hint = "run: xcode-select --install"
	case "windows":
		hint = "run: winget install --id Git.Git -e"
	}
	p.fail("git is not installed. To read your commits, " + hint)
	return false
}

// ensureGH offers to install the GitHub CLI and log in, and reports whether it is ready to use.
func ensureGH(ctx context.Context, p prompter) bool {
	if _, err := exec.LookPath("gh"); err != nil {
		install := ghInstallCommand()
		if install == nil {
			p.fail("GitHub CLI (gh) is not installed. Install it from https://cli.github.com to include your PRs and reviews")
			return false
		}
		if !p.confirm(fmt.Sprintf("GitHub CLI (gh) is not installed. It lists your PRs and reviews. Install it now with `%s`?", strings.Join(install, " ")), true) {
			return false
		}
		if err := interactive(ctx, install[0], install[1:]...); err != nil {
			p.fail("installing gh failed: " + err.Error())
			return false
		}
		if _, err := exec.LookPath("gh"); err != nil {
			p.fail("gh was installed but is not on PATH yet. Open a new terminal and run standup setup again")
			return false
		}
	}
	login, err := run.Output(ctx, "", nil, "gh", "api", "user", "--jq", ".login")
	if err != nil {
		if !p.confirm("gh is not logged in to GitHub. Log in now with `gh auth login`?", true) {
			return false
		}
		if err := interactive(ctx, "gh", "auth", "login"); err != nil {
			p.fail("gh auth login failed: " + err.Error())
			return false
		}
		if login, err = run.Output(ctx, "", nil, "gh", "api", "user", "--jq", ".login"); err != nil {
			p.fail("gh is still not logged in: " + err.Error())
			return false
		}
	}
	p.ok("gh is logged in as " + strings.TrimSpace(login))
	return true
}

func ghInstallCommand() []string {
	has := func(name string) bool { _, err := exec.LookPath(name); return err == nil }
	switch {
	case runtime.GOOS == "darwin" && has("brew"):
		return []string{"brew", "install", "gh"}
	case runtime.GOOS == "windows" && has("winget"):
		return []string{"winget", "install", "--id", "GitHub.cli", "-e"}
	case runtime.GOOS == "linux" && has("apt-get"):
		return []string{"sudo", "apt-get", "install", "-y", "gh"}
	case runtime.GOOS == "linux" && has("dnf"):
		return []string{"sudo", "dnf", "install", "-y", "gh"}
	case runtime.GOOS == "linux" && has("pacman"):
		return []string{"sudo", "pacman", "-S", "--noconfirm", "github-cli"}
	}
	return nil
}

// interactive runs a command attached to the terminal so it can prompt the user.
func interactive(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

// chooseSummarizer offers Claude Code and Codex. A custom command is still set by hand with agent = "command".
func chooseSummarizer(p prompter, s *config.Summarizer) error {
	var options []string
	def := 0
	for i, name := range summarize.Agents {
		status := "not installed"
		if _, err := exec.LookPath(name); err == nil {
			status = "installed"
			if s.Agent == "" && def == 0 {
				def = i + 1
			}
		}
		if s.Agent == name {
			def = i + 1
		}
		options = append(options, fmt.Sprintf("%s, %s", agents[name].label, status))
	}
	if def == 0 {
		def = 1
	}
	choice, err := p.choose("Which agent should write your standup? It runs without tools or file access.", options, def)
	if err != nil {
		return err
	}

	name := summarize.Agents[choice-1]
	info := agents[name]
	if _, err := exec.LookPath(name); err != nil {
		p.fail(name + " is not installed yet. " + info.install)
	}
	model := s.Model
	if s.Agent != name {
		model = info.defaultModel
	}
	s.Agent, s.Command = name, nil
	s.Model, err = chooseModel(p, info.models, model)
	return err
}

// chooseModel offers the agent's models, the current one when it is something else, and the agent's own default,
// which is saved as an empty model. With no models to offer it keeps the current one.
func chooseModel(p prompter, models []string, current string) (string, error) {
	if len(models) == 0 {
		return current, nil
	}
	options := slices.Clone(models)
	if current != "" && !slices.Contains(options, current) {
		options = append(options, current)
	}
	def := slices.Index(options, current) + 1
	options = append(options, "default (the agent picks)")
	if current == "" {
		def = len(options)
	}
	choice, err := p.choose("Model to use", options, def)
	if err != nil || choice == len(options) {
		return "", err
	}
	return options[choice-1], nil
}

func chooseSources(p prompter, cfg *config.Config, gitOK, ghOK bool) {
	home, _ := os.UserHomeDir()
	exists := func(path string) bool { _, err := os.Stat(path); return err == nil }
	claudeDir := filepath.Join(home, ".claude", "projects")
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		claudeDir = filepath.Join(d, "projects")
	}
	codexDir := filepath.Join(home, ".codex", "sessions")
	if d := os.Getenv("CODEX_HOME"); d != "" {
		codexDir = filepath.Join(d, "sessions")
	}
	available := []struct {
		name, label string
		ready       bool
	}{
		{"git", "git commits, branch switches and uncommitted work", gitOK},
		{"github", "GitHub PRs you opened, merged and reviewed", ghOK},
		{"claude", "Claude Code sessions", exists(claudeDir)},
		{"codex", "Codex sessions", exists(codexDir)},
	}
	var found []string
	for _, s := range available {
		if s.ready {
			p.ok(s.label)
			found = append(found, s.name)
		} else {
			p.fail(s.label + ": not available")
		}
	}
	if p.confirm("Use all the available sources?", true) {
		cfg.Sources = found
		return
	}
	cfg.Sources = nil
	for _, s := range available {
		if s.ready && p.confirm("Include "+s.label+"?", true) {
			cfg.Sources = append(cfg.Sources, s.name)
		}
	}
}

func chooseRepoDirs(p prompter, cfg *config.Config) {
	for {
		repos := activity.FindRepos(cfg.RepoDirs, cfg.RepoDepth)
		if len(cfg.RepoDirs) == 0 {
			p.fail("No repo folders found in the usual places")
		} else {
			p.ok(fmt.Sprintf("Found %d repos in %s", len(repos), strings.Join(cfg.RepoDirs, ", ")))
			if len(repos) > 0 && p.confirm("Use these folders?", true) {
				return
			}
		}
		answer := p.ask("Folders that hold your repos, separated by commas", strings.Join(cfg.RepoDirs, ", "))
		var dirs []string
		for _, d := range strings.Split(answer, ",") {
			if d = strings.TrimSpace(d); d != "" {
				dirs = append(dirs, config.ExpandHome(d))
			}
		}
		// Stop when the answer is empty or unchanged, so running out of input cannot loop forever
		if len(dirs) == 0 || strings.Join(dirs, ",") == strings.Join(cfg.RepoDirs, ",") {
			return
		}
		cfg.RepoDirs = dirs
	}
}
