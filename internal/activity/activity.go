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
	Counts   Counts
	// OutsideRepo is the repo standup ran in when repo_dirs does not cover it. Collect reads it anyway.
	OutsideRepo string

	hasGitHubPRs bool
}

// Counts tallies what Collect found, for the one-line summary shown before the agent runs.
type Counts struct {
	Commits int
	// Repos had commits, branch activity or uncommitted work in the window
	Repos   int
	PRs     int
	Reviews int
	// Prompts counts every prompt found, including any beyond the ones the activity log keeps
	Prompts int
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
		repos, outside := reposToRead(ctx, cfg.RepoDirs, cfg.RepoDepth, "")
		s, commits, active := gitSection(ctx, repos, w)
		mu.Lock()
		defer mu.Unlock()
		r.Git, r.OutsideRepo = s, outside
		r.Counts.Commits, r.Counts.Repos = commits, active
	})
	start("github", func() {
		s, prs, reviews, warnings := githubSection(ctx, w)
		mu.Lock()
		defer mu.Unlock()
		r.GitHub, r.hasGitHubPRs = s, prs+reviews > 0
		r.Counts.PRs, r.Counts.Reviews = prs, reviews
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
	r.Counts.Prompts = len(turns)
	r.Sessions = formatTurns(turns)
	return r
}

func (r Report) Empty() bool {
	return r.Git == "" && !r.hasGitHubPRs && r.Sessions == ""
}

// Summary describes in one line what Collect found in the enabled sources,
// such as "Found 12 commits across 4 repos, 3 PRs, 1 review and 41 agent prompts".
func (r Report) Summary() string {
	has := func(s string) bool { return config.Config{Sources: r.Sources}.HasSource(s) }
	var parts []string
	if has("git") {
		part := plural(r.Counts.Commits, "commit")
		if r.Counts.Repos > 0 {
			part += " across " + plural(r.Counts.Repos, "repo")
		}
		parts = append(parts, part)
	}
	if has("github") {
		parts = append(parts, plural(r.Counts.PRs, "PR"), plural(r.Counts.Reviews, "review"))
	}
	if has("claude") || has("codex") {
		parts = append(parts, plural(r.Counts.Prompts, "agent prompt"))
	}
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return "Found " + parts[0]
	}
	return "Found " + strings.Join(parts[:len(parts)-1], ", ") + " and " + parts[len(parts)-1]
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
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
