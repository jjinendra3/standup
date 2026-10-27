# standup

Type `standup` and get your daily standup, written from what you actually did: your git commits, your GitHub PRs and reviews, and your Claude Code and Codex sessions. The coding agent you already use writes it.

```
$ standup yesterday
Yesterday:
- Checkout: added retry with backoff to the payment webhook so duplicate events no longer
  double-charge, and opened acme/shop#412 for review.
- Reviewed acme/shop#409 (Move session storage to Redis).
Today:
- Finish the webhook retry tests; the idempotency key change is still uncommitted.
Blockers:
- None
```

## Install

Needs Go 1.22 or later.

```sh
go install github.com/jjinendra3/standup@latest
```

This puts `standup` in `$(go env GOPATH)/bin`, usually `~/go/bin`. Make sure that folder is on your `PATH`.

## Set up

```sh
standup setup
```

Setup walks you through:

1. **Tools.** Checks for git, offers to install the GitHub CLI (`gh`) and run `gh auth login`.
2. **Who writes the standup.** Claude Code, Codex, or any command that reads a prompt on stdin.
3. **Sources.** Shows what it found and lets you turn sources off.
4. **Repo folders.** Finds the folders that hold your repos, such as `~/code` or `~/Repos`.

It saves your answers to `~/.config/standup/config.toml`. You can skip setup: standup then uses the defaults it detects.

## Use

```sh
standup             # today so far
standup yesterday   # the previous working day; on a Monday, Friday to Sunday
standup y --copy    # also copy it to the clipboard
standup --context   # show the activity it collected, without summarizing
standup --prompt    # show the full prompt it would send
```

## What it reads

Everything is read locally. Nothing is uploaded except the final prompt, which goes to the agent you chose.

| Source | What | Where |
| --- | --- | --- |
| `git` | Your commits on any branch, branch switches, rebases and merges, and uncommitted changes touched in the window | Repos up to `repo_depth` folders below each of `repo_dirs` |
| `github` | PRs you opened, updated or merged, and other people's PRs you reviewed | `gh` CLI |
| `claude` | Each prompt you typed and Claude's last reply before your next prompt | `~/.claude/projects` (or `$CLAUDE_CONFIG_DIR`) |
| `codex` | Each prompt you typed and Codex's last reply | `~/.codex/sessions` (or `$CODEX_HOME`) |

Claude Code and Codex don't document their session formats. standup reads only a few fields and skips anything it doesn't recognise, but an agent update can still change what it finds. Run `standup --context` to see exactly what it collected.

## Who writes the summary

| `agent` | Runs | Notes |
| --- | --- | --- |
| `claude` | `claude -p --tools ""` | No tools, and no session saved |
| `codex` | `codex exec --sandbox read-only --ephemeral` | Read-only, and no session saved |
| `command` | Your `command`, with the prompt on stdin | For example `["llm", "-m", "gpt-5"]` or `["ollama", "run", "llama3"]` |

The agent runs in an empty temporary folder, so no project instructions get loaded. The activity log includes PR titles written by other people, so the agents run without tools, and the prompt tells them to treat the log as data.

## Settings

`~/.config/standup/config.toml`, or `%AppData%\standup\config.toml` on Windows. `$STANDUP_CONFIG` or `$XDG_CONFIG_HOME` override the location.

```toml
repo_dirs = ["~/code", "~/work"]
repo_depth = 2                       # ~/code/org/repo is depth 2
sources = ["git", "github", "claude", "codex"]
rules = ["Write in British English", "Start each bullet with the ticket ID when there is one"]

[summarizer]
agent = "claude"                     # claude, codex or command
model = "sonnet"                     # empty for the agent's default
command = []                         # for agent = "command"
```

## Platforms

CI runs the tests on macOS, Linux and Windows. The clipboard uses `pbcopy`, `clip`, or `wl-copy`/`xclip`/`xsel` on Linux.

### Raycast (macOS)

Raycast doesn't load your shell profile, so the script sets `PATH` itself. Save it in your Raycast script commands folder:

```sh
#!/bin/bash
# @raycast.schemaVersion 1
# @raycast.title Standup
# @raycast.mode silent
# @raycast.argument1 { "type": "text", "placeholder": "today | yesterday", "optional": true }
export PATH="$HOME/go/bin:/opt/homebrew/bin:/usr/local/bin:$PATH"
standup "${1:-today}" --copy 2>&1 >/dev/null | tail -1
```

If your agent needs anything else from your shell profile, export it here too, such as `NODE_EXTRA_CA_CERTS` behind a corporate proxy.

## Development

```sh
go test ./...
STANDUP_E2E_AGENT=codex go test ./internal/summarize -run EndToEnd   # calls the real agent with a dummy prompt
```

The last prompt sent is saved to `standup-prompt.txt` in your temp folder for debugging.

## License

MIT
