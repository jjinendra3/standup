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

// Claude Code keeps one JSONL transcript per session under ~/.claude/projects. The format is
// undocumented, so this reads only a few fields and skips anything it does not recognise.

func claudeDir() string {
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return filepath.Join(d, "projects")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "projects")
}

type claudeRecord struct {
	Type             string `json:"type"`
	Timestamp        string `json:"timestamp"`
	IsMeta           bool   `json:"isMeta"`
	IsSidechain      bool   `json:"isSidechain"`
	IsCompactSummary bool   `json:"isCompactSummary"`
	Cwd              string `json:"cwd"`
	GitBranch        string `json:"gitBranch"`
	Origin           *struct {
		Kind string `json:"kind"`
	} `json:"origin"`
	Message struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

func claudeTurns(w window.Window) ([]Turn, []string) {
	files := recentJSONL(w.Since, claudeDir())
	return readSessions(files, func(r io.Reader) ([]Turn, error) { return parseClaudeSession(r, w) })
}

func parseClaudeSession(r io.Reader, w window.Window) ([]Turn, error) {
	var turns []Turn
	var cur *Turn
	flush := func() {
		if cur != nil && w.Contains(cur.Time) {
			turns = append(turns, *cur)
		}
		cur = nil
	}
	err := eachLine(r, func(line []byte) {
		var rec claudeRecord
		if json.Unmarshal(line, &rec) != nil || rec.IsSidechain {
			return
		}
		switch rec.Type {
		case "user":
			prompt := claudePrompt(rec)
			if prompt == "" {
				return
			}
			t, err := time.Parse(time.RFC3339Nano, rec.Timestamp)
			if err != nil {
				return
			}
			flush()
			cur = &Turn{Time: t.Local(), Agent: "claude", Repo: repoName(rec.Cwd), Branch: rec.GitBranch, Prompt: prompt}
		case "assistant":
			if cur == nil {
				return
			}
			if text := strings.Join(blockTexts(rec.Message.Content, "text"), "\n"); strings.TrimSpace(text) != "" {
				cur.Reply = oneLine(text, maxReplyRunes)
			}
		}
	})
	flush()
	return turns, err
}

// claudePrompt returns what you typed in a user record, or "" for tool results, injected context and system messages.
func claudePrompt(rec claudeRecord) string {
	if rec.IsMeta || rec.IsCompactSummary || (rec.Origin != nil && rec.Origin.Kind != "human") {
		return ""
	}
	// A prompt is a string, or an array of blocks when sent from an IDE or with images
	var texts []string
	var s string
	if json.Unmarshal(rec.Message.Content, &s) == nil {
		texts = []string{s}
	} else {
		texts = blockTexts(rec.Message.Content, "text")
	}
	var kept []string
	for _, text := range texts {
		text = strings.TrimSpace(pastedContent.ReplaceAllString(text, "[pasted text]"))
		if text == "" || text == "[pasted text]" ||
			strings.HasPrefix(text, "<") || strings.HasPrefix(text, "[Request interrupted") || strings.HasPrefix(text, "Caveat:") {
			continue
		}
		kept = append(kept, text)
	}
	return oneLine(strings.Join(kept, " "), maxPromptRunes)
}
