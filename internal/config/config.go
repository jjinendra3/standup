// Package config loads and saves the standup settings file.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/BurntSushi/toml"
)

const (
	AgentClaude  = "claude"
	AgentCodex   = "codex"
	AgentCommand = "command"
)

// Sources lists every activity source standup can read.
var Sources = []string{"git", "github", "claude", "codex"}

type Summarizer struct {
	// Agent is claude, codex or command. Empty picks the first of claude and codex found on PATH.
	Agent string `toml:"agent"`
	// Model is passed to the agent. Empty uses the agent's own default.
	Model string `toml:"model"`
	// Command is the program run when Agent is command. It reads the prompt on stdin and prints the standup.
	Command []string `toml:"command"`
}

type Config struct {
	RepoDirs   []string   `toml:"repo_dirs"`
	RepoDepth  int        `toml:"repo_depth"`
	Sources    []string   `toml:"sources"`
	Rules      []string   `toml:"rules"`
	Summarizer Summarizer `toml:"summarizer"`
}

func (c Config) HasSource(name string) bool {
	for _, s := range c.Sources {
		if s == name {
			return true
		}
	}
	return false
}

// Path is $STANDUP_CONFIG, else $XDG_CONFIG_HOME/standup/config.toml, else ~/.config/standup/config.toml
// (the user config directory on Windows).
func Path() (string, error) {
	if p := os.Getenv("STANDUP_CONFIG"); p != "" {
		return p, nil
	}
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "standup", "config.toml"), nil
	}
	if runtime.GOOS == "windows" {
		d, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(d, "standup", "config.toml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "standup", "config.toml"), nil
}

// Load reads the settings file and fills in defaults. A missing file is not an error: found is false.
func Load() (cfg Config, found bool, err error) {
	path, err := Path()
	if err != nil {
		return cfg, false, err
	}
	_, err = toml.DecodeFile(path, &cfg)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		found = false
	case err != nil:
		return cfg, true, fmt.Errorf("reading %s: %w", path, err)
	default:
		found = true
	}
	cfg.fillDefaults()
	return cfg, found, nil
}

func (c *Config) fillDefaults() {
	if len(c.RepoDirs) == 0 {
		c.RepoDirs = DetectRepoDirs()
	}
	for i, d := range c.RepoDirs {
		c.RepoDirs[i] = ExpandHome(d)
	}
	if c.RepoDepth <= 0 {
		c.RepoDepth = 2
	}
	if len(c.Sources) == 0 {
		c.Sources = Sources
	}
}

// DetectRepoDirs returns the usual places people keep repos that exist in the home directory.
// It leaves out ~/Documents and ~/Desktop, which make macOS ask for folder access.
func DetectRepoDirs() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	var dirs []string
	var seen []os.FileInfo
next:
	for _, name := range []string{"Repos", "repos", "code", "Code", "src", "dev", "Developer", "Projects", "projects", "work", "workspace", "git", "GitHub"} {
		path := filepath.Join(home, name)
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			continue
		}
		// Case-insensitive file systems report ~/Repos and ~/repos as the same folder
		for _, s := range seen {
			if os.SameFile(s, info) {
				continue next
			}
		}
		seen = append(seen, info)
		dirs = append(dirs, path)
	}
	return dirs
}

// ExpandHome replaces a leading ~ with the home directory.
func ExpandHome(path string) string {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, strings.TrimPrefix(path, "~"))
}

// Save writes cfg to the settings file with comments explaining each setting, and returns its path.
func Save(cfg Config) (string, error) {
	path, err := Path()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("# standup settings. Run `standup setup` to change them interactively.\n\n")
	b.WriteString("# Folders that hold your git repos, searched repo_depth levels deep\n")
	fmt.Fprintf(&b, "repo_dirs = %s\nrepo_depth = %d\n\n", tomlList(cfg.RepoDirs), cfg.RepoDepth)
	b.WriteString("# Activity to include: git, github, claude, codex\n")
	fmt.Fprintf(&b, "sources = %s\n\n", tomlList(cfg.Sources))
	b.WriteString("# Extra instructions for the summary, for example [\"Write in British English\"]\n")
	fmt.Fprintf(&b, "rules = %s\n\n", tomlList(cfg.Rules))
	b.WriteString("[summarizer]\n")
	b.WriteString("# Which agent writes the standup: claude, codex or command\n")
	fmt.Fprintf(&b, "agent = %s\n", tomlString(cfg.Summarizer.Agent))
	b.WriteString("# Model passed to the agent. Leave empty for the agent's default\n")
	fmt.Fprintf(&b, "model = %s\n", tomlString(cfg.Summarizer.Model))
	b.WriteString("# For agent = \"command\": a program that reads the prompt on stdin and prints the standup\n")
	fmt.Fprintf(&b, "command = %s\n", tomlList(cfg.Summarizer.Command))
	return path, os.WriteFile(path, []byte(b.String()), 0o644)
}

func tomlList(items []string) string {
	quoted := make([]string, len(items))
	for i, s := range items {
		quoted[i] = tomlString(s)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

func tomlString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"' || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\u%04X`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
