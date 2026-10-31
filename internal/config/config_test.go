package config

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	t.Setenv("STANDUP_CONFIG", filepath.Join(t.TempDir(), "config.toml"))
	want := Config{
		RepoDirs:  []string{`C:\Users\me\code`, "/home/me/work dir"},
		RepoDepth: 3,
		Sources:   []string{"git", "codex"},
		Rules:     []string{`Say "shipped", not "deployed"`, "line one\nline two"},
		Summarizer: Summarizer{
			Agent:   AgentCommand,
			Model:   "",
			Command: []string{"llm", "-m", "gpt-5"},
		},
	}
	if _, err := Save(want); err != nil {
		t.Fatal(err)
	}
	got, found, err := Load()
	if err != nil || !found {
		t.Fatalf("Load: found=%v err=%v", found, err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("round trip changed the config\n got: %#v\nwant: %#v", got, want)
	}
}

func TestRepoDirFor(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	root := string(filepath.Separator)
	tests := []struct {
		name, repo, want string
	}{
		{"the folder that holds the repo", filepath.Join(home, "Personal", "standup"), filepath.Join(home, "Personal")},
		{"the repo itself when it sits in the home folder", filepath.Join(home, "standup"), filepath.Join(home, "standup")},
		{"the repo itself when it sits in a root", root + "standup", root + "standup"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RepoDirFor(tt.repo); got != tt.want {
				t.Errorf("RepoDirFor(%q) = %q, want %q", tt.repo, got, tt.want)
			}
		})
	}
}

func TestLoadMissingFileUsesDefaults(t *testing.T) {
	t.Setenv("STANDUP_CONFIG", filepath.Join(t.TempDir(), "missing.toml"))
	cfg, found, err := Load()
	if err != nil || found {
		t.Fatalf("Load: found=%v err=%v", found, err)
	}
	if cfg.RepoDepth != 2 || !reflect.DeepEqual(cfg.Sources, Sources) {
		t.Errorf("defaults not applied: %#v", cfg)
	}
}
