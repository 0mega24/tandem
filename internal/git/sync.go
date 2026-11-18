package git

import (
	"context"
	"errors"
	"fmt"
)

// ErrNotFastForward means the local branch has diverged from origin.
var ErrNotFastForward = errors.New("local branch has diverged from origin; not a fast-forward")

func (s System) Fetch(ctx context.Context, repo string) error {
	_, err := s.run(ctx, repo, "fetch", "--quiet", "--prune", "origin")
	return err
}

// FastForward moves branch to origin/<branch> after a Fetch.
//
// The caller must check first that the tree is clean when branch is checked out.
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
