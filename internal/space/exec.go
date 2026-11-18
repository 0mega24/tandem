package space

import (
	"context"
	"errors"
	"io"
	"os/exec"

	"github.com/0mega24/tandem/internal/state"
)

// ExecResult is the result of the command in one worktree.
type ExecResult struct {
	Entry state.RepoEntry
	Err   error // nil when the command exited 0
}

// Exec runs argv in each worktree of the space, one after the other, so the output of two repos
// does not mix. It goes on after a failure. before is called before each run, for a heading.
//
// Symlinked always.repos are skipped: they are the main clones, which a space must not change.
func (m *Manager) Exec(ctx context.Context, name string, argv []string, stdout, stderr io.Writer, before func(state.RepoEntry)) ([]ExecResult, error) {
	if len(argv) == 0 {
		return nil, errors.New("give a command")
	}

	sp, err := m.store.Load(name)
	if err != nil {
		return nil, err
	}

	var out []ExecResult
	for _, e := range sp.Repos {
		if e.Symlink {
			continue
		}
		if err := ctx.Err(); err != nil {
			return out, err // Ctrl-C stops the rest, not only the command that runs now
		}

		before(e)
		cmd := exec.CommandContext(ctx, argv[0], argv[1:]...) //nolint:gosec // G204: running the user's command is the purpose
		cmd.Dir = e.WorktreePath
		cmd.Stdout, cmd.Stderr = stdout, stderr
		out = append(out, ExecResult{Entry: e, Err: cmd.Run()})
	}
	return out, nil
}
