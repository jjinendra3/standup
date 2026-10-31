// Command standup writes your daily standup from git, GitHub and coding agent activity.
package main

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"slices"
	"syscall"
	"time"

	"github.com/jjinendra3/standup/internal/activity"
	"github.com/jjinendra3/standup/internal/clipboard"
	"github.com/jjinendra3/standup/internal/config"
	"github.com/jjinendra3/standup/internal/constants"
	"github.com/jjinendra3/standup/internal/picker"
	"github.com/jjinendra3/standup/internal/setup"
	"github.com/jjinendra3/standup/internal/spinner"
	"github.com/jjinendra3/standup/internal/summarize"
	"github.com/jjinendra3/standup/internal/window"
)

// version is set at build time with -ldflags "-X main.version=…"; go install falls back to the module version.
var version = "dev"

const summaryTimeout = 5 * time.Minute

// writingVerbs rotate under the spinner, in a random order, while the agent writes the standup.
var writingVerbs = []string{
	"Pondering…",
	"Connecting the dots…",
	"Reading between the commits…",
	"Untangling your branches…",
	"Translating git into English…",
	"Trimming the jargon…",
	"Finding the headline…",
	"Polishing the bullets…",
	"Hunting for blockers…",
	"Skipping the yak shaving…",
	"Making it sound intentional…",
	"Rehearsing for standup…",
	"Counting the wins…",
	"Squashing the details…",
	"Rebasing your thoughts…",
	"Resolving conflicts in prose…",
	"Consulting the rubber duck…",
	"Converting coffee into bullets…",
	"Blaming it on DNS…",
	"Turning it off and on again…",
	"Bribing the build server…",
	"Herding cats…",
	"Hiding the force-pushes…",
	"Pretending to read the RFC…",
	"Rolling a d20 for productivity…",
	"Sneaking past the scrum master…",
	"Muting your mic, just in case…",
	"Inflating your story points…",
	"Practising your camera-on face…",
	"Waiting for someone else to go first…",
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if len(args) > 0 && args[0] == "setup" {
		if err := setup.Run(ctx, os.Stdin, os.Stdout); err != nil {
			if errors.Is(err, picker.ErrInterrupted) {
				return interrupted()
			}
			return fail(err)
		}
		return 0
	}

	mode := window.Today
	copyToClipboard := false
	output := "summary"
	for _, a := range args {
		switch a {
		case "", "today", "t":
			mode = window.Today
		case "yesterday", "y":
			mode = window.Yesterday
		case "-c", "--copy":
			copyToClipboard = true
		case "--context":
			output = "context"
		case "--prompt":
			output = "prompt"
		case "-h", "--help", "help":
			fmt.Print(constants.Usage)
			if path, err := config.Path(); err == nil {
				fmt.Printf("\nSettings: %s\n", path)
			}
			return 0
		case "-v", "--version", "version":
			fmt.Println("standup " + versionString())
			return 0
		default:
			fmt.Fprintf(os.Stderr, "standup: unknown argument %q\n\n%s", a, constants.Usage)
			return 2
		}
	}

	// The agent runs in its own session, so stop on a closed terminal or a kill too, which cleans up its processes
	ctx, stopTerm := signal.NotifyContext(ctx, syscall.SIGHUP, syscall.SIGTERM)
	defer stopTerm()

	cfg, found, err := config.Load()
	if err != nil {
		return fail(err)
	}
	w := window.For(mode, time.Now())
	spin := startSpinner("Collecting your activity…")
	report := activity.Collect(ctx, cfg, w)
	spin.Stop()
	if ctx.Err() != nil {
		return interrupted()
	}
	for _, warning := range report.Warnings {
		fmt.Fprintln(os.Stderr, "standup: "+warning)
	}
	if report.OutsideRepo != "" {
		fmt.Fprintf(os.Stderr, "standup: also read %s, the repo you're in. To include it from any folder, add %s to repo_dirs or run standup setup\n",
			filepath.Base(report.OutsideRepo), config.RepoDirFor(report.OutsideRepo))
	}
	if report.Empty() {
		fmt.Fprintf(os.Stderr, "No activity found for %s\n", mode)
		if !found {
			fmt.Fprintln(os.Stderr, "Run standup setup to pick your repo folders and sources.")
		}
		return 0
	}
	fmt.Fprintln(os.Stderr, report.Summary())

	activityLog := report.Markdown()
	prompt := summarize.Prompt(w, cfg.Rules, activityLog)
	// Keep the last prompt around for debugging
	_ = os.WriteFile(filepath.Join(os.TempDir(), "standup-prompt.txt"), []byte(prompt), 0o600)

	switch output {
	case "context":
		fmt.Print(activityLog)
		return 0
	case "prompt":
		fmt.Print(prompt)
		return 0
	}

	agent, err := summarize.ResolveAgent(cfg.Summarizer)
	if err != nil {
		return fail(err)
	}
	// The blank line keeps the spinner, and then the standup, apart from the status above
	fmt.Fprintf(os.Stderr, "Writing your %s standup with %s…\n\n", mode, agent)
	summaryCtx, cancel := context.WithTimeout(ctx, summaryTimeout)
	defer cancel()
	verbs := slices.Clone(writingVerbs)
	rand.Shuffle(len(verbs), func(i, j int) { verbs[i], verbs[j] = verbs[j], verbs[i] })
	spin = startSpinner(verbs...)
	summary, err := summarize.Run(summaryCtx, cfg.Summarizer, prompt)
	spin.Stop()
	if ctx.Err() != nil {
		return interrupted()
	}
	if err != nil {
		return fail(fmt.Errorf("summary failed: %w", err))
	}
	fmt.Println(summary)

	if copyToClipboard {
		if err := clipboard.Copy(ctx, summary+"\n"); err != nil {
			return fail(fmt.Errorf("copying to the clipboard: %w", err))
		}
		fmt.Fprintf(os.Stderr, "Standup for %s copied\n", mode)
	}
	return 0
}

// startSpinner animates a status line on stderr while slow work runs. It returns nil, which Stop accepts,
// when stderr is not a terminal, so piped or redirected output stays clean.
func startSpinner(messages ...string) *spinner.Spinner {
	if !spinner.IsTerminal(os.Stderr) {
		return nil
	}
	return spinner.Start(os.Stderr, messages...)
}

// interrupted reports that standup was stopped and returns the exit code shells use for Ctrl-C.
func interrupted() int {
	fmt.Fprintln(os.Stderr, "standup: interrupted")
	return 130
}

func fail(err error) int {
	fmt.Fprintln(os.Stderr, "standup: "+err.Error())
	return 1
}

func versionString() string {
	if version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return version
}
