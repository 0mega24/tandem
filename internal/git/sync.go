package git

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ErrNotFastForward means the local branch has diverged from origin.
var ErrNotFastForward = errors.New("local branch has diverged from origin; not a fast-forward")

func (s System) Fetch(ctx context.Context, repo string) error {
	_, err := s.run(ctx, repo, "fetch", "--quiet", "--prune", "origin")
	return err
}

// FastForward moves branch to origin/<branch> after a Fetch. It reports whether the branch moved.
//
// The caller must check first that the tree is clean when branch is checked out.
func (s System) FastForward(ctx context.Context, repo, branch string) (bool, error) {
	remote := "refs/remotes/origin/" + branch
	local := "refs/heads/" + branch

	revs, err := s.line(ctx, repo, "rev-parse", local, remote)
	if err != nil {
		return false, err
	}

	old, target, _ := strings.Cut(revs, "\n")
	if old == target {
		return false, nil
	}

	_, err = s.run(ctx, repo, "merge-base", "--is-ancestor", local, remote)
	if exitCode(err) == 1 {
		// Local commits on top of origin are not a divergence: there is nothing to pull.
		_, err = s.run(ctx, repo, "merge-base", "--is-ancestor", remote, local)
		if err == nil {
			return false, nil
		}
		if exitCode(err) == 1 {
			return false, fmt.Errorf("%s in %s: %w", branch, repo, ErrNotFastForward)
		}
	}
	if err != nil {
		return false, err
	}

	head, err := s.line(ctx, repo, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err == nil && head == branch {
		_, err = s.run(ctx, repo, "merge", "--ff-only", "--quiet", target)
		return err == nil, err
	}

	// The old value makes update-ref fail if the branch moved since rev-parse.
	_, err = s.run(ctx, repo, "update-ref", "-m", "tdm repo sync: fast-forward", local, target, old)
	return err == nil, err
}
