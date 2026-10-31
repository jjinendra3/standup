// Package picker lets the user choose one of several options with the arrow keys on a terminal.
package picker

import (
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"unicode/utf8"

	"golang.org/x/term"
)

// ErrInterrupted is returned when the user presses Ctrl-C while choosing.
var ErrInterrupted = errors.New("interrupted")

type key int

const (
	keyOther key = iota
	keyUp
	keyDown
	keyEnter
	keyInterrupt
)

// Available reports whether in and out are an interactive terminal the picker can draw on.
// Windows consoles do not always process the cursor codes it redraws with, so callers ask there by number instead.
func Available(in, out *os.File) bool {
	return runtime.GOOS != "windows" && term.IsTerminal(int(in.Fd())) && term.IsTerminal(int(out.Fd()))
}

// Select draws options below the cursor with def (0-based) highlighted, lets the user move with the arrow keys
// or j/k, and returns the index chosen with Enter. tty is the terminal to read from; keys reads from it,
// so a buffered reader already wrapping tty keeps any input it holds.
func Select(tty *os.File, keys io.Reader, out io.Writer, options []string, def int) (int, error) {
	state, err := term.MakeRaw(int(tty.Fd()))
	if err != nil {
		return 0, err
	}
	defer term.Restore(int(tty.Fd()), state)
	width, _, err := term.GetSize(int(tty.Fd()))
	if err != nil {
		width = 0
	}
	return choose(keys, out, options, def, width)
}

// choose runs the picker on a terminal width columns wide, or of unknown width when it is 0.
func choose(keys io.Reader, out io.Writer, options []string, cur, width int) (int, error) {
	draw(out, options, cur, width)
	buf := make([]byte, 64)
	for {
		n, err := keys.Read(buf)
		if n == 0 && err != nil {
			return cur, err
		}
		for _, k := range parseKeys(buf[:n]) {
			switch k {
			case keyUp:
				cur = (cur - 1 + len(options)) % len(options)
			case keyDown:
				cur = (cur + 1) % len(options)
			case keyEnter:
				// Collapse the list to the chosen option
				fmt.Fprintf(out, "\x1b[%dA\x1b[J  ❯ %s\r\n", len(options), options[cur])
				return cur, nil
			case keyInterrupt:
				fmt.Fprintf(out, "\x1b[%dA\x1b[J", len(options))
				return cur, ErrInterrupted
			}
		}
		fmt.Fprintf(out, "\x1b[%dA", len(options))
		draw(out, options, cur, width)
	}
}

// gutter is the width of the "  ❯ " before each option.
const gutter = 4

// draw writes one line per option, marking the current one, and leaves the cursor below the list.
// Options are cut to fit the width, because a line that wraps would throw off moving back up to redraw.
// The terminal is in raw mode, so each line ends with an explicit carriage return.
func draw(out io.Writer, options []string, cur, width int) {
	for i, o := range options {
		marker := "  "
		if i == cur {
			marker = "❯ "
		}
		// One column spare, as some terminals wrap as soon as the last column is written
		fmt.Fprintf(out, "\r\x1b[2K  %s%s\r\n", marker, fit(o, width-gutter-1))
	}
}

// fit shortens s to n characters, ending in … when it cuts. n <= 0 leaves s whole.
func fit(s string, n int) string {
	if n <= 0 || utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n-1]) + "…"
}

// parseKeys turns raw terminal input into keys. One read can hold several keys when one is held down.
func parseKeys(b []byte) []key {
	var keys []key
	for i := 0; i < len(b); i++ {
		switch {
		// Arrows arrive as ESC [ A or, in application cursor mode, ESC O A
		case b[i] == 0x1b && i+2 < len(b) && (b[i+1] == '[' || b[i+1] == 'O'):
			switch b[i+2] {
			case 'A':
				keys = append(keys, keyUp)
			case 'B':
				keys = append(keys, keyDown)
			default:
				keys = append(keys, keyOther)
			}
			i += 2
		case b[i] == 'k':
			keys = append(keys, keyUp)
		case b[i] == 'j':
			keys = append(keys, keyDown)
		case b[i] == '\r' || b[i] == '\n':
			keys = append(keys, keyEnter)
		case b[i] == 0x03:
			keys = append(keys, keyInterrupt)
		default:
			keys = append(keys, keyOther)
		}
	}
	return keys
}
