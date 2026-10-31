package setup

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/jjinendra3/standup/internal/picker"
)

// prompter asks questions line by line. When input runs out, every question takes its default.
// When ctx is cancelled, by Ctrl-C, the question waiting for an answer returns ctx.Err().
type prompter struct {
	ctx context.Context
	in  *bufio.Reader
	out io.Writer
	tty *os.File // the terminal behind in, when choices can be picked with the arrow keys
}

func (p prompter) section(title string) { fmt.Fprintf(p.out, "\n%s\n", title) }
func (p prompter) ok(msg string)        { fmt.Fprintf(p.out, "  ✓ %s\n", msg) }
func (p prompter) fail(msg string)      { fmt.Fprintf(p.out, "  ✗ %s\n", msg) }

// readLine waits for the next line, or for ctx to be cancelled. A read from a terminal cannot be stopped,
// so after a cancel it is left waiting, and standup exits without it. Once ctx is cancelled no new read starts.
func (p prompter) readLine() (string, error) {
	if p.ctx.Err() == nil {
		line := make(chan string, 1)
		go func() {
			s, _ := p.in.ReadString('\n')
			line <- s
		}()
		select {
		case s := <-line:
			return strings.TrimSpace(s), nil
		case <-p.ctx.Done():
		}
	}
	// End the prompt line, which now shows the ^C the terminal echoed
	fmt.Fprintln(p.out)
	return "", p.ctx.Err()
}

func (p prompter) ask(question, def string) (string, error) {
	if def != "" {
		fmt.Fprintf(p.out, "  %s [%s]: ", question, def)
	} else {
		fmt.Fprintf(p.out, "  %s: ", question)
	}
	answer, err := p.readLine()
	if answer == "" {
		answer = def
	}
	return answer, err
}

func (p prompter) confirm(question string, def bool) (bool, error) {
	hint := "Y/n"
	if !def {
		hint = "y/N"
	}
	for {
		fmt.Fprintf(p.out, "  %s [%s]: ", question, hint)
		answer, err := p.readLine()
		if err != nil {
			return false, err
		}
		switch strings.ToLower(answer) {
		case "":
			return def, nil
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
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
		answer, err := p.ask("Choose", strconv.Itoa(def))
		if err != nil {
			return 0, err
		}
		if n, err := strconv.Atoi(answer); err == nil && n >= 1 && n <= len(options) {
			return n, nil
		}
	}
}
