package activity

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maxPromptRunes = 200
	maxReplyRunes  = 300
	maxTurns       = 80
)

// Turn is one prompt you sent a coding agent and the last thing it replied before your next prompt.
type Turn struct {
	Time   time.Time
	Agent  string
	Repo   string
	Branch string
	Prompt string
	Reply  string
}

func formatTurns(turns []Turn) string {
	sort.SliceStable(turns, func(i, j int) bool { return turns[i].Time.Before(turns[j].Time) })
	if len(turns) > maxTurns {
		turns = turns[len(turns)-maxTurns:]
	}
	var b strings.Builder
	for _, t := range turns {
		branch := t.Branch
		if branch == "" {
			branch = "-"
		}
		fmt.Fprintf(&b, "- %s %s %s (%s): %s\n", t.Time.Format("15:04"), t.Agent, t.Repo, branch, t.Prompt)
		if t.Reply != "" {
			fmt.Fprintf(&b, "  Reply: %s\n", t.Reply)
		}
	}
	return b.String()
}

var (
	// Claude Code inlines large pastes as <pasted_content id="…">…</pasted_content id="…">
	pastedContent = regexp.MustCompile(`(?s)<pasted_content[^>]*>.*?</pasted_content[^>]*>`)
	whitespace    = regexp.MustCompile(`\s+`)
)

// oneLine collapses whitespace and cuts s to max runes.
func oneLine(s string, max int) string {
	s = strings.TrimSpace(whitespace.ReplaceAllString(s, " "))
	if utf8.RuneCountInString(s) > max {
		s = string([]rune(s)[:max]) + "…"
	}
	return s
}

func repoName(cwd string) string {
	if cwd == "" {
		return "?"
	}
	return filepath.Base(filepath.Clean(cwd))
}

type textBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// blockTexts returns the text of the blocks of the given types in a message content array.
func blockTexts(raw json.RawMessage, types ...string) []string {
	var blocks []textBlock
	if json.Unmarshal(raw, &blocks) != nil {
		return nil
	}
	var texts []string
	for _, b := range blocks {
		for _, t := range types {
			if b.Type == t && b.Text != "" {
				texts = append(texts, b.Text)
			}
		}
	}
	return texts
}

// recentJSONL lists the .jsonl files under dirs modified at or after since. Missing dirs are skipped.
func recentJSONL(since time.Time, dirs ...string) []string {
	var files []string
	for _, dir := range dirs {
		_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".jsonl") {
				return nil
			}
			if info, err := d.Info(); err == nil && !info.ModTime().Before(since) {
				files = append(files, path)
			}
			return nil
		})
	}
	return files
}

// eachLine calls fn for every line of r. Lines can be megabytes long when they hold tool output.
func eachLine(r io.Reader, fn func([]byte)) error {
	br := bufio.NewReaderSize(r, 1<<20)
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			fn(line)
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// readSessions parses each file with parse and returns all turns, plus the files it could not read.
func readSessions(files []string, parse func(io.Reader) ([]Turn, error)) ([]Turn, []string) {
	var turns []Turn
	var failed []string
	for _, path := range files {
		f, err := os.Open(path)
		if err != nil {
			failed = append(failed, path)
			continue
		}
		t, err := parse(f)
		f.Close()
		if err != nil {
			failed = append(failed, path)
		}
		turns = append(turns, t...)
	}
	return turns, failed
}
