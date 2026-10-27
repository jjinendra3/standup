// Package clipboard copies text with the platform's clipboard command.
package clipboard

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/jjinendra3/standup/internal/run"
)

func Copy(ctx context.Context, text string) error {
	name, args, err := command()
	if err != nil {
		return err
	}
	_, err = run.Output(ctx, "", strings.NewReader(text), name, args...)
	return err
}

func command() (string, []string, error) {
	switch runtime.GOOS {
	case "darwin":
		return "pbcopy", nil, nil
	case "windows":
		return "clip", nil, nil
	}
	candidates := [][]string{{"xclip", "-selection", "clipboard"}, {"xsel", "--clipboard", "--input"}}
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		candidates = append([][]string{{"wl-copy"}}, candidates...)
	}
	for _, c := range candidates {
		if _, err := exec.LookPath(c[0]); err == nil {
			return c[0], c[1:], nil
		}
	}
	return "", nil, errors.New("no clipboard command found. Install wl-clipboard, xclip or xsel")
}
