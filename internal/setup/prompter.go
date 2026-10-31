package setup

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/jjinendra3/standup/internal/picker"
)

// prompter asks questions line by line. When input runs out, every question takes its default.
type prompter struct {
	in  *bufio.Reader
	out io.Writer
	tty *os.File // the terminal behind in, when choices can be picked with the arrow keys
}

func (p prompter) section(title string) { fmt.Fprintf(p.out, "\n%s\n", title) }
func (p prompter) ok(msg string)        { fmt.Fprintf(p.out, "  ✓ %s\n", msg) }
func (p prompter) fail(msg string)      { fmt.Fprintf(p.out, "  ✗ %s\n", msg) }

func (p prompter) readLine() string {
	line, _ := p.in.ReadString('\n')
	return strings.TrimSpace(line)
}

func (p prompter) ask(question, def string) string {
	if def != "" {
		fmt.Fprintf(p.out, "  %s [%s]: ", question, def)
	} else {
		fmt.Fprintf(p.out, "  %s: ", question)
	}
	if answer := p.readLine(); answer != "" {
		return answer
	}
	return def
}

func (p prompter) confirm(question string, def bool) bool {
	hint := "Y/n"
	if !def {
		hint = "y/N"
	}
	for {
		fmt.Fprintf(p.out, "  %s [%s]: ", question, hint)
		switch strings.ToLower(p.readLine()) {
		case "":
			return def
		case "y", "yes":
			return true
		case "n", "no":
			return false
		}
	}
}

// choose returns the 1-based choice among options: picked with the arrow keys on a terminal, else typed as a number.
// It returns picker.ErrInterrupted when the user presses Ctrl-C in the picker.
func (p prompter) choose(question string, options []string, def int) (int, error) {
	if p.tty != nil {
		fmt.Fprintf(p.out, "  %s  (↑/↓, Enter)\n", question)
		i, err := picker.Select(p.tty, p.in, p.out, options, def-1)
		if err == nil || errors.Is(err, picker.ErrInterrupted) {
			return i + 1, err
		}
	} else {
		fmt.Fprintf(p.out, "  %s\n", question)
	}
	for i, o := range options {
		fmt.Fprintf(p.out, "    %d) %s\n", i+1, o)
	}
	for {
		answer := p.ask("Choose", strconv.Itoa(def))
		if n, err := strconv.Atoi(answer); err == nil && n >= 1 && n <= len(options) {
			return n, nil
		}
	}
}
