package git

import (
	"context"
	"strings"
)

// Runner is the set of git operations that Tandem uses. System is the real one; tests of the
// space operations use a fake to inject a failure at one step and check the rollback.
type Runner interface {
	OriginURL(ctx context.Context, repo string) (string, error)
	DefaultBranch(ctx context.Context, repo string) (string, error)
	BranchExists(ctx context.Context, repo, branch string) (bool, error)
	DeleteBranch(ctx context.Context, repo, branch string, force bool) error

	AddWorktree(ctx context.Context, repo, path, branch, from string) error
	Worktrees(ctx context.Context, repo string) ([]Worktree, error)
	RemoveWorktree(ctx context.Context, repo, path string, force bool) error
	PruneWorktrees(ctx context.Context, repo string) error
	RepairWorktree(ctx context.Context, repo, path string) error
}

// OriginURL is the URL of the origin remote, or "" when the repo has no origin.
func (s System) OriginURL(ctx context.Context, repo string) (string, error) {
	url, err := s.line(ctx, repo, "remote", "get-url", "origin")
	if exitCode(err) == 2 { // git uses 2 for "no such remote"
		return "", nil
	}
	return url, err
}

// DefaultBranch is the branch that origin/HEAD points to. Without origin/HEAD (a repo that was
// not cloned, or `git remote set-head` was never run) it is the branch checked out in repo.
func (s System) DefaultBranch(ctx context.Context, repo string) (string, error) {
	ref, err := s.line(ctx, repo, "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD")
	if err == nil {
		return strings.TrimPrefix(ref, "origin/"), nil
	}
	if exitCode(err) != 1 { // 1 = not a symbolic ref
		return "", err
	}
	return s.line(ctx, repo, "symbolic-ref", "--short", "HEAD")
}

// BranchExists reports whether refs/heads/<branch> exists in repo.
func (s System) BranchExists(ctx context.Context, repo, branch string) (bool, error) {
	_, err := s.run(ctx, repo, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch)
	if err == nil {
		return true, nil
	}
	if exitCode(err) == 1 {
		return false, nil
	}
	return false, err
}

// DeleteBranch deletes a local branch. Without force, git refuses a branch that is not merged.
func (s System) DeleteBranch(ctx context.Context, repo, branch string, force bool) error {
	flag := "-d"
	if force {
		flag = "-D"
	}
	_, err := s.run(ctx, repo, "branch", flag, "--", branch)
	return err
}
