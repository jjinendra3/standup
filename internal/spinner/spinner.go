// Package spinner animates a one-line status on a terminal while slow work runs.
package spinner

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	frameEvery   = 100 * time.Millisecond
	messageEvery = 3 * time.Second
)

// Braille dots draw a smooth circle on macOS and Linux; older Windows console fonts lack them.
var frames = func() []string {
	if runtime.GOOS == "windows" {
		return []string{"|", "/", "-", `\`}
	}
	return []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
}()

// Spinner redraws one status line in place until Stop. Methods on a nil *Spinner do nothing.
type Spinner struct {
	stop chan struct{}
	done chan struct{}
	once sync.Once
}

// IsTerminal reports whether f is an interactive terminal, so callers can skip the animation
// when output is piped or redirected.
func IsTerminal(f *os.File) bool {
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// Start draws the status line on w, showing each message in turn for a few seconds, followed by the time elapsed.
// Nothing else should write to w until Stop returns.
func Start(w io.Writer, messages ...string) *Spinner {
	s := &Spinner{stop: make(chan struct{}), done: make(chan struct{})}
	go s.run(w, messages, time.Now())
	return s
}

// Stop clears the status line and returns once the spinner has stopped writing. It is safe to call more than once.
func (s *Spinner) Stop() {
	if s == nil {
		return
	}
	s.once.Do(func() { close(s.stop) })
	<-s.done
}

func (s *Spinner) run(w io.Writer, messages []string, start time.Time) {
	defer close(s.done)
	ticker := time.NewTicker(frameEvery)
	defer ticker.Stop()
	width := 0
	for i := 0; ; i++ {
		width = redraw(w, line(i, time.Since(start), messages), width)
		select {
		case <-s.stop:
			redraw(w, "", width)
			fmt.Fprint(w, "\r")
			return
		case <-ticker.C:
		}
	}
}

// line renders frame i of the status line once elapsed has passed.
func line(i int, elapsed time.Duration, messages []string) string {
	text := frames[i%len(frames)]
	if len(messages) > 0 {
		text += " " + messages[int(elapsed/messageEvery)%len(messages)]
	}
	return text + " " + elapsed.Truncate(time.Second).String()
}

// redraw replaces the current line with text and returns its width. Spaces cover any longer previous line;
// a carriage return and spaces work on every terminal, unlike ANSI erase codes.
func redraw(w io.Writer, text string, prevWidth int) int {
	width := utf8.RuneCountInString(text)
	fmt.Fprint(w, "\r"+text+strings.Repeat(" ", max(prevWidth-width, 0)))
	return width
}
