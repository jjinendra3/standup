package activity

import (
	"strings"
	"testing"
	"time"

	"github.com/jjinendra3/standup/internal/window"
)

// testWindow covers 2026-10-21 in UTC so fixture timestamps do not depend on the machine's time zone.
var testWindow = window.Window{
	Since: time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC),
	Until: time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC),
}

func lines(l ...string) string { return strings.Join(l, "\n") + "\n" }

func TestParseClaudeSession(t *testing.T) {
	transcript := lines(
		// Before the window: starts a turn that must not be reported
		`{"type":"user","timestamp":"2026-10-20T23:59:00.000Z","cwd":"/r/api","gitBranch":"main","message":{"content":"old prompt"}}`,
		`{"type":"assistant","timestamp":"2026-10-20T23:59:30.000Z","message":{"content":[{"type":"text","text":"old reply"}]}}`,
		// Plain string prompt; the last assistant text before the next prompt is the reply
		`{"type":"user","timestamp":"2026-10-21T09:00:00.000Z","cwd":"/r/api","gitBranch":"feat/login","message":{"content":"add the login endpoint"}}`,
		`{"type":"assistant","timestamp":"2026-10-21T09:00:05.000Z","message":{"content":[{"type":"text","text":"Reading the router first."}]}}`,
		`{"type":"assistant","timestamp":"2026-10-21T09:00:06.000Z","message":{"content":[{"type":"tool_use","name":"Read"}]}}`,
		`{"type":"user","timestamp":"2026-10-21T09:00:07.000Z","message":{"content":[{"type":"tool_result","content":"file body"}]}}`,
		`{"type":"assistant","timestamp":"2026-10-21T09:01:00.000Z","message":{"content":[{"type":"text","text":"Added POST /login\nand its tests."}]}}`,
		// Not prompts: meta, sidechain, compaction summary, task notification, interruption, command wrapper
		`{"type":"user","timestamp":"2026-10-21T09:02:00.000Z","isMeta":true,"message":{"content":"meta"}}`,
		`{"type":"user","timestamp":"2026-10-21T09:02:00.000Z","isSidechain":true,"message":{"content":"subagent task"}}`,
		`{"type":"user","timestamp":"2026-10-21T09:02:00.000Z","isCompactSummary":true,"message":{"content":"This session is being continued"}}`,
		`{"type":"user","timestamp":"2026-10-21T09:02:00.000Z","origin":{"kind":"task-notification"},"message":{"content":"task done"}}`,
		`{"type":"user","timestamp":"2026-10-21T09:02:00.000Z","message":{"content":[{"type":"text","text":"[Request interrupted by user]"}]}}`,
		`{"type":"user","timestamp":"2026-10-21T09:02:00.000Z","message":{"content":"<command-name>/clear</command-name>"}}`,
		`not json`,
		// IDE prompt: context blocks dropped, pasted content collapsed with its id-carrying closing tag
		`{"type":"user","timestamp":"2026-10-21T10:00:00.000Z","cwd":"/r/web","gitBranch":"dev","origin":{"kind":"human"},"message":{"content":[{"type":"text","text":"<ide_opened_file>a.ts</ide_opened_file>"},{"type":"text","text":"why does this fail <pasted_content id=\"ab12\">\nstack\ntrace\n</pasted_content id=\"ab12\">"}]}}`,
		// After the window
		`{"type":"user","timestamp":"2026-10-22T00:00:00.000Z","cwd":"/r/api","message":{"content":"tomorrow"}}`,
	)
	turns, err := parseClaudeSession(strings.NewReader(transcript), testWindow)
	if err != nil {
		t.Fatal(err)
	}
	want := []Turn{
		{Agent: "claude", Repo: "api", Branch: "feat/login", Prompt: "add the login endpoint", Reply: "Added POST /login and its tests."},
		{Agent: "claude", Repo: "web", Branch: "dev", Prompt: "why does this fail [pasted text]"},
	}
	assertTurns(t, turns, want)
}

func TestParseCodexSession(t *testing.T) {
	session := lines(
		`{"timestamp":"2026-10-21T08:00:00.000Z","type":"session_meta","payload":{"cwd":"/r/api","source":"vscode","git":{"branch":"feat/x"}}}`,
		`{"timestamp":"2026-10-21T08:00:01.000Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"<environment_context><cwd>/r/api</cwd></environment_context>"},{"type":"input_text","text":"# AGENTS.md instructions for /r/api\n\nbe nice"}]}}`,
		`{"timestamp":"2026-10-21T08:00:02.000Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"# Context from my IDE setup:\n\n## Active file: a.go\n\n## My request for Codex:\nfix the flaky test\n"}]}}`,
		`{"timestamp":"2026-10-21T08:00:03.000Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Looking."}]}}`,
		`{"timestamp":"2026-10-21T08:05:00.000Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Fixed the race in TestCache."}]}}`,
		`{"timestamp":"2026-10-21T08:06:00.000Z","type":"turn_context","payload":{"cwd":"/r/web"}}`,
		`{"timestamp":"2026-10-21T08:06:01.000Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"now the web side"}]}}`,
	)
	turns, err := parseCodexSession(strings.NewReader(session), testWindow)
	if err != nil {
		t.Fatal(err)
	}
	assertTurns(t, turns, []Turn{
		{Agent: "codex", Repo: "api", Branch: "feat/x", Prompt: "fix the flaky test", Reply: "Fixed the race in TestCache."},
		{Agent: "codex", Repo: "web", Branch: "feat/x", Prompt: "now the web side"},
	})
}

func TestParseCodexSessionSkipsSubagents(t *testing.T) {
	session := lines(
		`{"timestamp":"2026-10-21T08:00:00.000Z","type":"session_meta","payload":{"cwd":"/r/api","source":{"subagent":{"other":"guardian"}}}}`,
		`{"timestamp":"2026-10-21T08:00:01.000Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"review this action"}]}}`,
	)
	turns, err := parseCodexSession(strings.NewReader(session), testWindow)
	if err != nil || len(turns) != 0 {
		t.Fatalf("got %d turns, err %v; want none", len(turns), err)
	}
}

func TestOneLineTruncatesByRune(t *testing.T) {
	if got := oneLine("  héllo\n\n  wörld  ", 7); got != "héllo w…" {
		t.Errorf("got %q", got)
	}
}

func assertTurns(t *testing.T, got, want []Turn) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d turns, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		g := got[i]
		g.Time = time.Time{}
		if g != want[i] {
			t.Errorf("turn %d:\n got %+v\nwant %+v", i, g, want[i])
		}
	}
}
