package activity

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jjinendra3/standup/internal/window"
)

// isolateGit stops the developer's own git config, such as hooks or signing, from affecting the test repos.
func isolateGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	empty := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", empty)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
}

func git(t *testing.T, dir string, env []string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func newRepo(t *testing.T, path, email string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, path, nil, "init", "-q", "-b", "main")
	git(t, path, nil, "config", "user.name", "Me")
	git(t, path, nil, "config", "user.email", email)
	git(t, path, nil, "config", "commit.gpgsign", "false")
}

func commit(t *testing.T, repo, file, msg string, extraEnv ...string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repo, file), []byte(msg), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, repo, nil, "add", file)
	git(t, repo, extraEnv, "commit", "-q", "-m", msg)
}

func TestFindRepos(t *testing.T) {
	isolateGit(t)
	root := t.TempDir()
	for _, dir := range []string{"api", "org/web", "org/team/too-deep", "node_modules/pkg", ".cache/hidden"} {
		newRepo(t, filepath.Join(root, filepath.FromSlash(dir)), "me@example.com")
	}
	var names []string
	for _, r := range FindRepos([]string{root}, 2) {
		names = append(names, r.Name)
	}
	if got := strings.Join(names, ","); got != "api,org/web" {
		t.Errorf("found %q, want api,org/web", got)
	}
}

func TestReposToRead(t *testing.T) {
	isolateGit(t)
	root, elsewhere := t.TempDir(), t.TempDir()
	covered := filepath.Join(root, "api")
	newRepo(t, covered, "me@example.com")
	src := filepath.Join(elsewhere, "side-project", "src")
	newRepo(t, filepath.Dir(src), "me@example.com")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name, dir, wantNames, wantOutside string
	}{
		{"the repo you are in is added when the roots miss it", src, "api,side-project", "side-project"},
		{"a repo the roots cover is not added twice", covered, "api", ""},
		{"a folder outside any repo adds nothing", elsewhere, "api", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repos, outside := reposToRead(context.Background(), []string{root}, 2, tt.dir)
			var names []string
			for _, r := range repos {
				names = append(names, r.Name)
			}
			if got := strings.Join(names, ","); got != tt.wantNames {
				t.Errorf("repos %q, want %q", got, tt.wantNames)
			}
			if outside != "" {
				outside = filepath.Base(outside)
			}
			if outside != tt.wantOutside {
				t.Errorf("outside %q, want %q", outside, tt.wantOutside)
			}
		})
	}
}

func TestGitSection(t *testing.T) {
	isolateGit(t)
	root := t.TempDir()
	repo := filepath.Join(root, "api")
	newRepo(t, repo, "me@example.com")
	// git log --since stops at the first commit older than the window, so the old commit goes first as in real history
	commit(t, repo, "c.txt", "chore: ancient", "GIT_COMMITTER_DATE=2020-01-01T00:00:00Z", "GIT_AUTHOR_DATE=2020-01-01T00:00:00Z")
	commit(t, repo, "a.txt", "feat: add login")
	commit(t, repo, "b.txt", "fix: teammate change", "GIT_AUTHOR_EMAIL=someone@example.com", "GIT_COMMITTER_EMAIL=someone@example.com")
	git(t, repo, nil, "checkout", "-q", "-b", "feat/signup")
	if err := os.WriteFile(filepath.Join(repo, "wip.txt"), []byte("draft"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, commits, repos := gitSection(context.Background(), FindRepos([]string{root}, 2), window.For(window.Today, time.Now()))
	if commits != 1 || repos != 1 {
		t.Errorf("counted %d commits in %d repos, want 1 in 1", commits, repos)
	}
	for _, want := range []string{
		"### api\n",
		// Both branches point at this commit, so either name is a valid source
		"- feat: add login  [",
		"- checkout: moving from main to feat/signup",
		"Uncommitted work on branch feat/signup",
		"?? wip.txt",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	for _, unwanted := range []string{"teammate change", "ancient"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("should not include %q:\n%s", unwanted, out)
		}
	}
}

func TestGitSectionSkipsCommitsWithoutEmail(t *testing.T) {
	isolateGit(t)
	root := t.TempDir()
	repo := filepath.Join(root, "api")
	newRepo(t, repo, "me@example.com")
	commit(t, repo, "a.txt", "feat: someone else's work")
	git(t, repo, nil, "config", "--unset", "user.email")

	out, commits, _ := gitSection(context.Background(), FindRepos([]string{root}, 2), window.For(window.Today, time.Now()))
	if strings.Contains(out, "Commits by me") || commits != 0 {
		t.Errorf("an empty author must not match every commit, counted %d:\n%s", commits, out)
	}
}
