// Command standup writes your daily standup from git, GitHub and coding agent activity.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"time"

	"github.com/jjinendra3/standup/internal/activity"
	"github.com/jjinendra3/standup/internal/clipboard"
	"github.com/jjinendra3/standup/internal/config"
	"github.com/jjinendra3/standup/internal/constants"
	"github.com/jjinendra3/standup/internal/setup"
	"github.com/jjinendra3/standup/internal/summarize"
	"github.com/jjinendra3/standup/internal/window"
)

// version is set at build time with -ldflags "-X main.version=…"; go install falls back to the module version.
var version = "dev"

const summaryTimeout = 5 * time.Minute

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if len(args) > 0 && args[0] == "setup" {
		if err := setup.Run(ctx, os.Stdin, os.Stdout); err != nil {
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

	cfg, found, err := config.Load()
	if err != nil {
		return fail(err)
	}
	w := window.For(mode, time.Now())
	report := activity.Collect(ctx, cfg, w)
	for _, warning := range report.Warnings {
		fmt.Fprintln(os.Stderr, "standup: "+warning)
	}
	if report.Empty() {
		fmt.Fprintf(os.Stderr, "No activity found for %s\n", mode)
		if !found {
			fmt.Fprintln(os.Stderr, "Run standup setup to pick your repo folders and sources.")
		}
		return 0
	}

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
	fmt.Fprintf(os.Stderr, "Writing your %s standup with %s…\n", mode, agent)
	summaryCtx, cancel := context.WithTimeout(ctx, summaryTimeout)
	defer cancel()
	summary, err := summarize.Run(summaryCtx, cfg.Summarizer, prompt)
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
