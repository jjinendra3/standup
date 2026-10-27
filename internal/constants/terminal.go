package constants

const Usage = `Usage: standup [today|yesterday] [options]
       standup setup

Writes your standup from git commits, GitHub PRs and reviews, and your Claude Code
and Codex sessions, summarized by the coding agent you choose.

  today, t        activity since midnight (default)
  yesterday, y    the previous working day; on a Monday this covers Friday to Sunday
  setup           choose the agent, install gh and pick your repo folders

Options:
  -c, --copy      also copy the standup to the clipboard
      --context   print the collected activity and exit, without summarizing
      --prompt    print the full prompt that would be sent to the agent
  -h, --help      show this help
  -v, --version   show the version
`
