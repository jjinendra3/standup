// Package activity collects what you worked on in a time window from git, GitHub and coding agent sessions.
package activity

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/jjinendra3/standup/internal/config"
	"github.com/jjinendra3/standup/internal/window"
)

type Report struct {
	Window   window.Window
	Sources  []string
	Git      string
	GitHub   string
	Sessions string
	// Warnings describe sources that were skipped or only partly read
	Warnings []string

	hasGitHubPRs bool
}

// Collect reads every source enabled in cfg in parallel. A failing source adds a warning instead of failing the report.
func Collect(ctx context.Context, cfg config.Config, w window.Window) Report {
	r := Report{Window: w, Sources: cfg.Sources}
	var mu sync.Mutex
	var wg sync.WaitGroup
	var turns []Turn
	start := func(source string, fn func()) {
		if !cfg.HasSource(source) {
			return
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			fn()
		}()
	}
	addTurns := func(agent string, t []Turn, failed []string) {
		mu.Lock()
		defer mu.Unlock()
		turns = append(turns, t...)
		if len(failed) > 0 {
			r.Warnings = append(r.Warnings, fmt.Sprintf("could not read %d %s session files", len(failed), agent))
		}
	}

	start("git", func() {
		s := gitSection(ctx, cfg.RepoDirs, cfg.RepoDepth, w)
		mu.Lock()
		defer mu.Unlock()
		r.Git = s
	})
	start("github", func() {
		s, found, warnings := githubSection(ctx, w)
		mu.Lock()
		defer mu.Unlock()
		r.GitHub, r.hasGitHubPRs = s, found
		r.Warnings = append(r.Warnings, warnings...)
	})
	start("claude", func() {
		t, failed := claudeTurns(w)
		addTurns("Claude Code", t, failed)
	})
	start("codex", func() {
		t, failed := codexTurns(w)
		addTurns("Codex", t, failed)
	})
	wg.Wait()
	r.Sessions = formatTurns(turns)
	return r
}

func (r Report) Empty() bool {
	return r.Git == "" && !r.hasGitHubPRs && r.Sessions == ""
}

// Markdown renders the report as the activity log the summarizer reads.
func (r Report) Markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Activity window: %s to %s (local time)\n",
		r.Window.Since.Format("2006-01-02 15:04"), r.Window.Until.Format("2006-01-02 15:04"))
	has := func(s string) bool { return config.Config{Sources: r.Sources}.HasSource(s) }
	if has("git") {
		b.WriteString("\n## Git\n" + r.Git)
	}
	if has("github") {
		b.WriteString("\n## GitHub\n" + r.GitHub)
	}
	if has("claude") || has("codex") {
		b.WriteString("\n## My coding agent sessions: time, agent, repo (branch), my prompt, and the agent's last reply\n" + r.Sessions)
	}
	return b.String()
}
