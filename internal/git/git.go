// Package git runs the system git binary. Tandem does not use go-git: worktree support there is
// not complete, and the user's git has the user's config and credential helpers.
//
// Output is parsed only from formats that git keeps stable: --porcelain, rev-parse, symbolic-ref
// and remote get-url.
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

// run runs git in dir and returns stdout.
func (System) run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...) //nolint:gosec // G204: the arguments are built by this package, not by a shell
	cmd.Dir = dir
	// No prompts: a fetch that needs a password fails instead of waiting on a terminal that the
	// TUI owns. LC_ALL=C keeps the stderr checks below stable.
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
