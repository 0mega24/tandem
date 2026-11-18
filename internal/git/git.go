// Package git runs the system git binary, so the user's git config and credential helpers apply.
// It parses only stable output formats (porcelain, rev-parse, symbolic-ref).
package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Error is a failed git command.
type Error struct {
	Dir    string
	Args   []string
	Stderr string
	Err    error
}

func (e *Error) Error() string {
	msg := strings.TrimSpace(e.Stderr)
	if msg == "" {
		msg = e.Err.Error()
	}
	return fmt.Sprintf("git %s (in %s): %s", strings.Join(e.Args, " "), e.Dir, msg)
}

func (e *Error) Unwrap() error { return e.Err }

// System runs the git found on PATH.
type System struct{}

var _ Runner = System{}

func (System) run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	//nolint:gosec // G204: this package builds the args; no shell is involved.
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir

	// Never prompt: the TUI owns the terminal. LC_ALL=C keeps stderr matching stable.
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "LC_ALL=C")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, &Error{Dir: dir, Args: args, Stderr: stderr.String(), Err: err}
	}
	return stdout.Bytes(), nil
}

// line runs git and returns stdout without the trailing newline.
func (s System) line(ctx context.Context, dir string, args ...string) (string, error) {
	out, err := s.run(ctx, dir, args...)
	return strings.TrimRight(string(out), "\n"), err
}

// exitCode is the exit status of a failed git command, or -1 for other errors.
func exitCode(err error) int {
	if ee, ok := errors.AsType[*exec.ExitError](err); ok {
		return ee.ExitCode()
	}
	return -1
}
