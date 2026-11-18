package git

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Runner is the set of git operations that Tandem uses.
//
// System is the real one. Space tests use a fake to make one step fail and check the rollback.
type Runner interface {
	OriginURL(ctx context.Context, repo string) (string, error)
	DefaultBranch(ctx context.Context, repo string) (string, error)
	BranchExists(ctx context.Context, repo, branch string) (bool, error)
	CheckBranchName(ctx context.Context, repo, branch string) error
	DeleteBranch(ctx context.Context, repo, branch string, force bool) error

	AddWorktree(ctx context.Context, repo, path, branch, from string) error
	Worktrees(ctx context.Context, repo string) ([]Worktree, error)
	RemoveWorktree(ctx context.Context, repo, path string, force bool) error
	PruneWorktrees(ctx context.Context, repo string) error
	RepairWorktree(ctx context.Context, repo, path string) error

	Status(ctx context.Context, dir string) (Status, error)
	Unpushed(ctx context.Context, repo, branch string) (int, error)

	Fetch(ctx context.Context, repo string) error
	FastForward(ctx context.Context, repo, branch string) (bool, error)
}

// OriginURL returns "" when the repo has no origin.
func (s System) OriginURL(ctx context.Context, repo string) (string, error) {
	url, err := s.line(ctx, repo, "remote", "get-url", "origin")
	if exitCode(err) == 2 { // git uses 2 for "no such remote"
		return "", nil
	}
	return url, err
}

// DefaultBranch is origin/HEAD, or the checked-out branch when origin/HEAD is not set.
//
// The fallback covers a repo that was made with `git init` and pushed later: it has no
// origin/HEAD, and its main clone is almost always on its default branch. If it is on a
// feature branch, that branch is treated as the default; on a detached HEAD this fails.
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

// BranchExists checks local branches only.
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

// CheckBranchName fails when git does not allow branch as a branch name.
func (s System) CheckBranchName(ctx context.Context, repo, branch string) error {
	_, err := s.run(ctx, repo, "check-ref-format", "--branch", branch)
	if _, ok := errors.AsType[*Error](err); ok { // git ran and refused the name ("-x" too)
		return fmt.Errorf("%q is not a valid branch name", branch)
	}
	return err
}

// DeleteBranch refuses an unmerged branch unless force is set.
func (s System) DeleteBranch(ctx context.Context, repo, branch string, force bool) error {
	flag := "-d"
	if force {
		flag = "-D"
	}
	_, err := s.run(ctx, repo, "branch", flag, "--", branch)
	return err
}
