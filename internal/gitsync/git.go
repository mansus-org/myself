package gitsync

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ErrNoGit is returned when the git binary is not on PATH.
var ErrNoGit = errors.New("git not found on PATH")

// Git runs git commands in one directory (git -C Dir).
type Git struct {
	Dir string
}

// Run runs git with args and returns trimmed stdout. A failure carries
// git's stderr in the error.
func (g Git) Run(args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", g.Dir}, args...)...)
	// Never block a cron job on a credential prompt or an editor.
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_MERGE_AUTOEDIT=no")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	out := strings.TrimSpace(stdout.String())
	if err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			return out, err
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = out
		}
		return out, fmt.Errorf("git %s: %s", args[0], firstLines(msg, 3))
	}
	return out, nil
}

// Ok reports whether git exits 0 for args.
func (g Git) Ok(args ...string) bool {
	_, err := g.Run(args...)
	return err == nil
}

func firstLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "; ")
}

// Available reports an error if git is not installed.
func Available() error {
	if _, err := exec.LookPath("git"); err != nil {
		return ErrNoGit
	}
	return nil
}
