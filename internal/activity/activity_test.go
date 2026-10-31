package activity

import "testing"

func TestSummary(t *testing.T) {
	all := []string{"git", "github", "claude", "codex"}
	tests := []struct {
		name    string
		sources []string
		counts  Counts
		want    string
	}{
		{"every source", all, Counts{Commits: 12, Repos: 4, PRs: 3, Reviews: 2, Prompts: 41},
			"Found 12 commits across 4 repos, 3 PRs, 2 reviews and 41 agent prompts"},
		{"singular nouns", all, Counts{Commits: 1, Repos: 1, PRs: 1, Reviews: 1, Prompts: 1},
			"Found 1 commit across 1 repo, 1 PR, 1 review and 1 agent prompt"},
		{"zeros still show, so a broken source stands out", all, Counts{Commits: 5, Repos: 2, Prompts: 9},
			"Found 5 commits across 2 repos, 0 PRs, 0 reviews and 9 agent prompts"},
		{"no active repos drops the repo count", []string{"git", "claude"}, Counts{Prompts: 3},
			"Found 0 commits and 3 agent prompts"},
		{"only enabled sources", []string{"git"}, Counts{Commits: 2, Repos: 1}, "Found 2 commits across 1 repo"},
		{"either agent counts as prompts", []string{"codex"}, Counts{Prompts: 4}, "Found 4 agent prompts"},
		{"no sources", nil, Counts{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Report{Sources: tt.sources, Counts: tt.counts}
			if got := r.Summary(); got != tt.want {
				t.Errorf("Summary() = %q, want %q", got, tt.want)
			}
		})
	}
}
