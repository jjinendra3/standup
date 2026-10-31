package picker

import (
	"bytes"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
)

func TestChoose(t *testing.T) {
	options := []string{"a", "b", "c"}
	tests := []struct {
		name  string
		input []string // one string per read
		def   int
		want  int
	}{
		{"enter takes the default", []string{"\r"}, 1, 1},
		{"down then enter", []string{"\x1b[B", "\r"}, 0, 1},
		{"up from the top wraps to the bottom", []string{"\x1b[A", "\r"}, 0, 2},
		{"down from the bottom wraps to the top", []string{"\x1b[B", "\n"}, 2, 0},
		{"application cursor mode arrows", []string{"\x1bOB\x1bOB", "\r"}, 0, 2},
		{"j and k move too", []string{"jjk", "\r"}, 0, 1},
		{"several keys in one read", []string{"\x1b[B\x1b[B\r"}, 0, 2},
		{"other keys are ignored", []string{"x2\x1b[C", "\r"}, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			got, err := choose(&reads{chunks: tt.input}, &out, options, tt.def)
			if err != nil || got != tt.want {
				t.Fatalf("choose() = %d, %v, want %d", got, err, tt.want)
			}
			if want := "\x1b[3A\x1b[J  ❯ " + options[tt.want] + "\r\n"; !strings.HasSuffix(out.String(), want) {
				t.Errorf("output %q does not end with the collapsed choice %q", out.String(), want)
			}
		})
	}
}

func TestChooseInterrupted(t *testing.T) {
	var out bytes.Buffer
	if _, err := choose(&reads{chunks: []string{"\x1b[B", "\x03"}}, &out, []string{"a", "b"}, 0); !errors.Is(err, ErrInterrupted) {
		t.Fatalf("err = %v, want ErrInterrupted", err)
	}
	if !strings.HasSuffix(out.String(), "\x1b[2A\x1b[J") {
		t.Errorf("output %q does not end by clearing the list", out.String())
	}
}

func TestChooseEndOfInput(t *testing.T) {
	got, err := choose(&reads{chunks: []string{"\x1b[B"}}, io.Discard, []string{"a", "b"}, 0)
	if !errors.Is(err, io.EOF) || got != 1 {
		t.Errorf("choose() = %d, %v, want 1, EOF", got, err)
	}
}

func TestDrawMarksTheCurrentOption(t *testing.T) {
	var out bytes.Buffer
	draw(&out, []string{"a", "b"}, 1)
	if got, want := out.String(), "\r\x1b[2K    a\r\n\r\x1b[2K  ❯ b\r\n"; got != want {
		t.Errorf("draw wrote %q, want %q", got, want)
	}
}

func TestParseKeysIgnoresACutOffEscape(t *testing.T) {
	// A lone ESC or a cut-off sequence is not an arrow
	if got := parseKeys([]byte("\x1b[")); !slices.Equal(got, []key{keyOther, keyOther}) {
		t.Errorf("parseKeys = %v", got)
	}
}

// reads returns one chunk per Read call, like a terminal delivering keys as they are pressed.
type reads struct{ chunks []string }

func (r *reads) Read(p []byte) (int, error) {
	if len(r.chunks) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.chunks[0])
	r.chunks = r.chunks[1:]
	return n, nil
}
