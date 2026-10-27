package activity

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jjinendra3/standup/internal/window"
)

// Codex keeps one JSONL rollout per session under ~/.codex/sessions/YYYY/MM/DD. The format is
// undocumented, so this reads only the session metadata and the user and assistant messages.

func codexDirs() []string {
	home := os.Getenv("CODEX_HOME")
	if home == "" {
		h, _ := os.UserHomeDir()
		home = filepath.Join(h, ".codex")
	}
	return []string{filepath.Join(home, "sessions"), filepath.Join(home, "archived_sessions")}
}

type codexRecord struct {
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

type codexPayload struct {
	Type    string          `json:"type"`
	Role    string          `json:"role"`
	Cwd     string          `json:"cwd"`
	Source  json.RawMessage `json:"source"`
	Content json.RawMessage `json:"content"`
	Git     *struct {
		Branch string `json:"branch"`
	} `json:"git"`
}

// codexRequestMarker precedes the typed request when Codex wraps it with IDE context
const codexRequestMarker = "## My request for Codex:"

func codexTurns(w window.Window) ([]Turn, []string) {
	files := recentJSONL(w.Since, codexDirs()...)
	return readSessions(files, func(r io.Reader) ([]Turn, error) { return parseCodexSession(r, w) })
}

func parseCodexSession(r io.Reader, w window.Window) ([]Turn, error) {
	var turns []Turn
	var cur *Turn
	var cwd, branch string
	skip := false
	flush := func() {
		if cur != nil && w.Contains(cur.Time) {
			turns = append(turns, *cur)
		}
		cur = nil
	}
	err := eachLine(r, func(line []byte) {
		var rec codexRecord
		var p codexPayload
		if skip || json.Unmarshal(line, &rec) != nil || json.Unmarshal(rec.Payload, &p) != nil {
			return
		}
		switch rec.Type {
		case "session_meta":
			// Subagent and reviewer sessions have an object source; your own sessions have a string like "cli" or "vscode"
			var source string
			if len(p.Source) > 0 && json.Unmarshal(p.Source, &source) != nil {
				skip = true
				return
			}
			cwd = p.Cwd
			if p.Git != nil {
				branch = p.Git.Branch
			}
		case "turn_context":
			if p.Cwd != "" {
				cwd = p.Cwd
			}
		case "response_item":
			if p.Type != "message" {
				return
			}
			switch p.Role {
			case "user":
				prompt := codexPrompt(blockTexts(p.Content, "input_text"))
				t, err := time.Parse(time.RFC3339Nano, rec.Timestamp)
				if prompt == "" || err != nil {
					return
				}
				flush()
				cur = &Turn{Time: t.Local(), Agent: "codex", Repo: repoName(cwd), Branch: branch, Prompt: prompt}
			case "assistant":
				if cur == nil {
					return
				}
				if text := strings.Join(blockTexts(p.Content, "output_text"), "\n"); strings.TrimSpace(text) != "" {
					cur.Reply = oneLine(text, maxReplyRunes)
				}
			}
		}
	})
	flush()
	return turns, err
}

// codexPrompt keeps what you typed and drops the context blocks Codex adds to user messages,
// such as <environment_context>, AGENTS.md instructions and "# Files mentioned by the user".
func codexPrompt(texts []string) string {
	var kept []string
	for _, text := range texts {
		if i := strings.Index(text, codexRequestMarker); i >= 0 {
			text = text[i+len(codexRequestMarker):]
		} else if t := strings.TrimSpace(text); strings.HasPrefix(t, "<") || strings.HasPrefix(t, "# ") ||
			strings.HasPrefix(t, "[Request interrupted") {
			continue
		}
		text = strings.TrimSpace(pastedContent.ReplaceAllString(text, "[pasted text]"))
		if text != "" && text != "[pasted text]" {
			kept = append(kept, text)
		}
	}
	return oneLine(strings.Join(kept, " "), maxPromptRunes)
}
