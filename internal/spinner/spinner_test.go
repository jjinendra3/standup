package spinner

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLine(t *testing.T) {
	messages := []string{"Pondering…", "Drafting…"}
	tests := []struct {
		name     string
		i        int
		elapsed  time.Duration
		messages []string
		want     string
	}{
		{"first frame and message", 0, 0, messages, frames[0] + " Pondering… 0s"},
		{"frames advance each tick", 1, 500 * time.Millisecond, messages, frames[1] + " Pondering… 0s"},
		{"frames wrap around", len(frames), 0, messages, frames[0] + " Pondering… 0s"},
		{"messages rotate with time", 0, messageEvery, messages, frames[0] + " Drafting… 5s"},
		{"messages wrap around", 0, 2*messageEvery + time.Second, messages, frames[0] + " Pondering… 11s"},
		{"no messages shows only the time", 0, 61 * time.Second, nil, frames[0] + " 1m1s"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := line(tt.i, tt.elapsed, tt.messages); got != tt.want {
				t.Errorf("line() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRedrawCoversALongerPreviousLine(t *testing.T) {
	var b bytes.Buffer
	if width := redraw(&b, "⠋ ab", 7); width != 4 {
		t.Errorf("width = %d, want 4", width)
	}
	if got, want := b.String(), "\r⠋ ab   "; got != want {
		t.Errorf("wrote %q, want %q", got, want)
	}
}

func TestStopClearsTheLine(t *testing.T) {
	var b bytes.Buffer
	s := Start(&b, "Pondering…")
	s.Stop()
	s.Stop() // a second Stop must not panic or block

	// The output is "\r<line>…\r<spaces>\r": blanks as wide as the last line plus an echoed ^C, then back to column 0.
	segments := strings.Split(b.String(), "\r")
	if len(segments) < 4 || segments[len(segments)-1] != "" {
		t.Fatalf("output %q does not end with a cleared line", b.String())
	}
	last := strings.TrimRight(segments[len(segments)-3], " ")
	blank := segments[len(segments)-2]
	if !strings.Contains(last, "Pondering…") {
		t.Errorf("last line %q does not show the message", last)
	}
	if strings.TrimSpace(blank) != "" || len([]rune(blank)) != len([]rune(last))+len("^C") {
		t.Errorf("clearing segment %q does not blank %q", blank, last)
	}
}

func TestStopOnNil(t *testing.T) {
	var s *Spinner
	s.Stop()
}

func TestIsTerminal(t *testing.T) {
	f, err := os.Create(filepath.Join(t.TempDir(), "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if IsTerminal(f) {
		t.Error("a regular file reported as a terminal")
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	if IsTerminal(w) {
		t.Error("a pipe reported as a terminal")
	}
}
