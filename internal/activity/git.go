package activity

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jjinendra3/standup/internal/run"
	"github.com/jjinendra3/standup/internal/window"
)

const (
	maxBranchActivity = 30
	maxStatusLines    = 20
	gitParallelism    = 8
)

type Repo struct {
	Path string
	// Name is the path relative to the folder it was found in, such as "org/api"
	Name string
}

// FindRepos returns the git repos under roots, at most depth levels below each root.
// It skips node_modules and hidden folders.
func FindRepos(roots []string, depth int) []Repo {
	seen := map[string]bool{}
	var repos []Repo
	for _, root := range roots {
		root, err := filepath.EvalSymlinks(root)
		if err != nil {
			continue
		}
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || !d.IsDir() {
				return nil
			}
			if path != root && (d.Name() == "node_modules" || strings.HasPrefix(d.Name(), ".")) {
				return filepath.SkipDir
			}
			if _, err := os.Lstat(filepath.Join(path, ".git")); err == nil && !seen[path] {
				seen[path] = true
				name, _ := filepath.Rel(root, path)
				if name == "." {
					name = filepath.Base(path)
				}
				repos = append(repos, Repo{Path: path, Name: filepath.ToSlash(name)})
			}
			if rel, _ := filepath.Rel(root, path); rel != "." && strings.Count(rel, string(filepath.Separator))+1 >= depth {
				return filepath.SkipDir
			}
			return nil
		})
	}
	sort.Slice(repos, func(i, j int) bool { return repos[i].Name < repos[j].Name })
	return repos
}

func gitSection(ctx context.Context, roots []string, depth int, w window.Window) string {
	repos := FindRepos(roots, depth)
	results := make([]string, len(repos))
	sem := make(chan struct{}, gitParallelism)
	var wg sync.WaitGroup
	for i, r := range repos {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i] = repoActivity(ctx, r, w)
		}()
	}
	wg.Wait()
	var b strings.Builder
	for _, s := range results {
		b.WriteString(s)
	}
	return b.String()
}

func repoActivity(ctx context.Context, r Repo, w window.Window) string {
	git := func(args ...string) string {
		out, _ := run.Output(ctx, "", nil, "git", append([]string{"-C", r.Path}, args...)...)
		return out
	}
	commits := myCommits(git, w)
	activity := branchActivity(git("reflog", "--date=unix", "--format=%gd%x09%gs"), w)
	wip := uncommitted(git, r.Path, w, len(commits)+len(activity) > 0)
	if len(commits)+len(activity) == 0 && wip == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("### " + r.Name + "\n")
	if len(commits) > 0 {
		b.WriteString("Commits by me [branch]:\n" + strings.Join(commits, "\n") + "\n")
	}
	if len(activity) > 0 {
		b.WriteString("Branch activity, oldest first:\n" + strings.Join(activity, "\n") + "\n")
	}
	if wip != "" {
		b.WriteString("Uncommitted work " + wip + "\n")
	}
	b.WriteString("\n")
	return b.String()
}

func myCommits(git func(...string) string, w window.Window) []string {
	// An empty --author matches everyone, so skip commits when no email is configured
	email := strings.TrimSpace(git("config", "user.email"))
	if email == "" {
		return nil
	}
	out := git("log", "--branches", "--remotes", "--source", "--no-merges", "--fixed-strings", "--author="+email,
		"--since="+w.Since.Format(time.RFC3339), "--until="+w.Until.Format(time.RFC3339), "--format=%S%x09%s")
	remotes := strings.Fields(git("remote"))
	// Rebased commits show up on both the local and remote branch, so dedupe by subject
	seen := map[string]bool{}
	var commits []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		ref, subject, ok := strings.Cut(line, "\t")
		if !ok || seen[subject] {
			continue
		}
		seen[subject] = true
		commits = append(commits, "- "+subject+"  ["+shortRef(ref, remotes)+"]")
	}
	return commits
}

func shortRef(ref string, remotes []string) string {
	ref = strings.TrimPrefix(ref, "refs/heads/")
	ref = strings.TrimPrefix(ref, "refs/remotes/")
	for _, r := range remotes {
		if strings.HasPrefix(ref, r+"/") {
			return strings.TrimPrefix(ref, r+"/")
		}
	}
	return ref
}

var reflogAction = regexp.MustCompile(`^(checkout:|rebase( -i)? \(finish\)|merge |pull|reset:|cherry-pick)`)

// branchActivity picks branch switches, rebases, merges, pulls and resets from reflog output
// (newest first, "HEAD@{unix time}<TAB>subject") and returns them oldest first.
func branchActivity(reflog string, w window.Window) []string {
	var newestFirst []string
	for _, line := range strings.Split(strings.TrimSpace(reflog), "\n") {
		selector, subject, ok := strings.Cut(line, "\t")
		if !ok || !reflogAction.MatchString(subject) {
			continue
		}
		epoch, err := strconv.ParseInt(strings.TrimSuffix(strings.TrimPrefix(selector, "HEAD@{"), "}"), 10, 64)
		if err != nil || !w.Contains(time.Unix(epoch, 0)) {
			continue
		}
		newestFirst = append(newestFirst, subject)
	}
	seen := map[string]bool{}
	var activity []string
	for i := len(newestFirst) - 1; i >= 0 && len(activity) < maxBranchActivity; i-- {
		if s := newestFirst[i]; !seen[s] {
			seen[s] = true
			activity = append(activity, "- "+s)
		}
	}
	return activity
}

// uncommitted describes changes in the working tree. They only count if the repo was active in the
// window or a changed file was modified since the window began, so stale changes stay out.
func uncommitted(git func(...string) string, repoPath string, w window.Window, active bool) string {
	fields := strings.Split(git("status", "--porcelain=v1", "-z"), "\x00")
	var lines []string
	var latest time.Time
	for i := 0; i < len(fields); i++ {
		entry := fields[i]
		if len(entry) < 4 {
			continue
		}
		status, path := entry[:2], entry[3:]
		// Renames and copies are followed by the original path
		if status[0] == 'R' || status[0] == 'C' {
			i++
		}
		lines = append(lines, status+" "+path)
		if info, err := os.Lstat(filepath.Join(repoPath, filepath.FromSlash(path))); err == nil && info.ModTime().After(latest) {
			latest = info.ModTime()
		}
	}
	if len(lines) == 0 || (!active && latest.Before(w.Since)) {
		return ""
	}
	header := "on branch " + strings.TrimSpace(git("branch", "--show-current"))
	if stat := strings.TrimSpace(git("diff", "--shortstat", "HEAD")); stat != "" {
		header += ", " + stat
	}
	if len(lines) > maxStatusLines {
		lines = lines[:maxStatusLines]
	}
	return header + "\n" + strings.Join(lines, "\n")
}
