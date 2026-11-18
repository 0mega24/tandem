package git

import (
	"context"
	"errors"
	"fmt"
)

// ErrNotFastForward means that the local branch has commits that origin does not have.
var ErrNotFastForward = errors.New("local branch has diverged from origin; not a fast-forward")

// Fetch updates the origin remote-tracking branches of repo and prunes deleted ones.
func (s System) Fetch(ctx context.Context, repo string) error {
	_, err := s.run(ctx, repo, "fetch", "--quiet", "--prune", "origin")
	return err
}

// FastForward moves the local branch to origin/<branch> after a Fetch. When branch is checked out
// in repo, it merges with --ff-only, so the working tree follows; the caller checks first that the
// tree is clean. Otherwise it moves only the ref. It returns ErrNotFastForward when the branches
// have diverged, and does nothing when the branch is already up to date.
func (s System) FastForward(ctx context.Context, repo, branch string) error {
	remote := "refs/remotes/origin/" + branch
	local := "refs/heads/" + branch
	_, err := s.run(ctx, repo, "merge-base", "--is-ancestor", local, remote)
	if exitCode(err) == 1 {
		return fmt.Errorf("%s in %s: %w", branch, repo, ErrNotFastForward)
	}
	if err != nil {
		return err
	}
	head, err := s.line(ctx, repo, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err == nil && head == branch {
		_, err = s.run(ctx, repo, "merge", "--ff-only", "--quiet", remote)
		return err
	}
	old, err := s.line(ctx, repo, "rev-parse", local)
	if err != nil {
		return err
	}
	// The old value makes update-ref fail if the branch moved since rev-parse.
	_, err = s.run(ctx, repo, "update-ref", "-m", "tdm repo sync: fast-forward", local, remote, old)
	return err
}
