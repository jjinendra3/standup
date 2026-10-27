package activity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/jjinendra3/standup/internal/run"
	"github.com/jjinendra3/standup/internal/window"
)

const ghSearchLimit = "30"

type ghPR struct {
	Repository struct {
		NameWithOwner string `json:"nameWithOwner"`
	} `json:"repository"`
	Number  int    `json:"number"`
	Title   string `json:"title"`
	State   string `json:"state"`
	IsDraft bool   `json:"isDraft"`
	Author  struct {
		Login string `json:"login"`
	} `json:"author"`
}

func (p ghPR) ref() string { return fmt.Sprintf("%s#%d", p.Repository.NameWithOwner, p.Number) }

// githubSection lists your PRs and reviews in the window using the gh CLI. found reports whether any PR was listed.
func githubSection(ctx context.Context, w window.Window) (section string, found bool, warnings []string) {
	if _, err := exec.LookPath("gh"); err != nil {
		return "", false, []string{"skipping GitHub: gh is not installed. Run standup setup, or install it from https://cli.github.com"}
	}
	out, err := run.Output(ctx, "", nil, "gh", "api", "user", "--jq", ".login")
	if err != nil {
		return "", false, []string{"skipping GitHub: gh is not logged in. Run gh auth login"}
	}
	me := strings.TrimSpace(out)
	rng := w.Since.UTC().Format(time.RFC3339) + ".." + w.Until.UTC().Format(time.RFC3339)

	// Each search takes a few seconds, so run them in parallel
	var authored, merged, reviewed []string
	var mu sync.Mutex
	warn := func(err error) {
		mu.Lock()
		defer mu.Unlock()
		warnings = append(warnings, "GitHub: "+err.Error())
	}
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		prs, err := searchPRs(ctx, "repository,number,title,state,isDraft", "--author=@me", "--updated="+rng)
		if err != nil {
			warn(err)
		}
		for _, p := range prs {
			state := p.State
			if p.IsDraft {
				state = "draft"
			}
			authored = append(authored, fmt.Sprintf("- %s [%s] %s", p.ref(), state, p.Title))
		}
	}()
	go func() {
		defer wg.Done()
		prs, err := searchPRs(ctx, "repository,number,title", "--author=@me", "--merged-at="+rng)
		if err != nil {
			warn(err)
		}
		for _, p := range prs {
			merged = append(merged, fmt.Sprintf("- %s %s", p.ref(), p.Title))
		}
	}()
	go func() {
		defer wg.Done()
		reviewed = reviewedPRs(ctx, me, rng, w, warn)
	}()
	wg.Wait()

	var b strings.Builder
	for _, s := range []struct {
		heading string
		items   []string
	}{
		{"PRs I authored, updated in window", authored},
		{"PRs I authored, merged in window", merged},
		{"Other people's PRs I reviewed in window", reviewed},
	} {
		b.WriteString("### " + s.heading + "\n")
		for _, item := range s.items {
			b.WriteString(item + "\n")
		}
		found = found || len(s.items) > 0
	}
	return b.String(), found, warnings
}

func searchPRs(ctx context.Context, fields string, qualifiers ...string) ([]ghPR, error) {
	args := append([]string{"search", "prs", "--limit", ghSearchLimit, "--json", fields}, qualifiers...)
	out, err := run.Output(ctx, "", nil, "gh", args...)
	if err != nil {
		return nil, err
	}
	var prs []ghPR
	if err := json.Unmarshal([]byte(out), &prs); err != nil {
		return nil, fmt.Errorf("reading gh search output: %w", err)
	}
	return prs, nil
}

// reviewedPRs lists other people's PRs you reviewed in the window. Search only knows you reviewed
// a PR at some point, so it checks each review's own timestamp.
func reviewedPRs(ctx context.Context, me, rng string, w window.Window, warn func(error)) []string {
	prs, err := searchPRs(ctx, "repository,number,title,author", "--reviewed-by=@me", "--updated="+rng)
	if err != nil {
		warn(err)
		return nil
	}
	results := make([]string, len(prs))
	var wg sync.WaitGroup
	for i, p := range prs {
		if p.Author.Login == me {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := reviewedInWindow(ctx, p, me, w)
			if err != nil {
				warn(err)
			}
			if ok {
				results[i] = fmt.Sprintf("- %s %s", p.ref(), p.Title)
			}
		}()
	}
	wg.Wait()
	var reviewed []string
	for _, r := range results {
		if r != "" {
			reviewed = append(reviewed, r)
		}
	}
	return reviewed
}

func reviewedInWindow(ctx context.Context, p ghPR, me string, w window.Window) (bool, error) {
	out, err := run.Output(ctx, "", nil, "gh", "api", fmt.Sprintf("repos/%s/pulls/%d/reviews?per_page=100", p.Repository.NameWithOwner, p.Number))
	if err != nil {
		return false, err
	}
	var reviews []struct {
		User struct {
			Login string `json:"login"`
		} `json:"user"`
		SubmittedAt time.Time `json:"submitted_at"`
	}
	if err := json.Unmarshal([]byte(out), &reviews); err != nil {
		return false, errors.New("reading reviews for " + p.ref() + ": " + err.Error())
	}
	for _, r := range reviews {
		if r.User.Login == me && w.Contains(r.SubmittedAt) {
			return true, nil
		}
	}
	return false, nil
}
