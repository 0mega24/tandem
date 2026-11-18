package git

import (
	"context"
	"errors"
	"strings"
)

// ErrRepairUnsupported means that git is older than 2.29, which added `git worktree repair`.
var ErrRepairUnsupported = errors.New("git worktree repair needs git 2.29 or newer")

// Worktree is one entry of `git worktree list --porcelain`.
type Worktree struct {
	Path     string
	Head     string // commit; empty in a new repo with no commits
	Branch   string // short name; empty when detached or bare
	Bare     bool
	Detached bool
	Prunable bool // git knows the entry, but its directory is gone
}

// AddWorktree makes a worktree at path on branch. With from set, it makes branch from that start
// point; else branch must exist and must not be checked out in another worktree.
func (s System) AddWorktree(ctx context.Context, repo, path, branch, from string) error {
	args := []string{"worktree", "add", "--quiet"}
	if from != "" {
		args = append(args, "-b", branch, "--", path, from)
	} else {
		args = append(args, "--", path, branch)
	}
	_, err := s.run(ctx, repo, args...)
	return err
}

// Worktrees lists the worktrees of repo, the main worktree first.
func (s System) Worktrees(ctx context.Context, repo string) ([]Worktree, error) {
	out, err := s.run(ctx, repo, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return nil, err
	}
	return parseWorktrees(string(out)), nil
}

// parseWorktrees reads the -z porcelain format: NUL after each line, an empty line between entries.
func parseWorktrees(out string) []Worktree {
	var list []Worktree
	var cur *Worktree
	for line := range strings.SplitSeq(out, "\x00") {
		key, value, _ := strings.Cut(line, " ")
		switch key {
		case "":
			cur = nil
		case "worktree":
			list = append(list, Worktree{Path: value})
			cur = &list[len(list)-1]
		}
		if cur == nil {
			continue
		}
		switch key {
		case "HEAD":
			cur.Head = value
		case "branch":
			cur.Branch = strings.TrimPrefix(value, "refs/heads/")
		case "bare":
			cur.Bare = true
		case "detached":
			cur.Detached = true
		case "prunable":
			cur.Prunable = true
		}
	}
	return list
}

// RemoveWorktree removes the worktree at path. Without force, git refuses a worktree with
// changes; the space operations ask the user before they force.
func (s System) RemoveWorktree(ctx context.Context, repo, path string, force bool) error {
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	_, err := s.run(ctx, repo, append(args, "--", path)...)
	return err
}

// PruneWorktrees deletes the git entries of worktrees whose directories are gone.
func (s System) PruneWorktrees(ctx context.Context, repo string) error {
	_, err := s.run(ctx, repo, "worktree", "prune")
	return err
}

// RepairWorktree connects a worktree directory that git lost track of (for example after a move)
// back to repo. It returns ErrRepairUnsupported on git older than 2.29.
func (s System) RepairWorktree(ctx context.Context, repo, path string) error {
	_, err := s.run(ctx, repo, "worktree", "repair", "--", path)
	if gerr, ok := errors.AsType[*Error](err); ok && strings.Contains(gerr.Stderr, "usage: git worktree") {
		return ErrRepairUnsupported // old git prints the usage for an unknown subcommand
	}
	return err
}
