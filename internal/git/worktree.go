package git

import (
	"context"
	"errors"
	"strings"
)

// ErrRepairUnsupported is returned by git older than 2.29.
var ErrRepairUnsupported = errors.New("git worktree repair needs git 2.29 or newer")

// Worktree is one entry of the worktree list.
type Worktree struct {
	Path     string
	Head     string
	Branch   string // short name; empty when detached or bare
	Bare     bool
	Detached bool
	Prunable bool // the directory is gone
}

// AddWorktree checks out branch at path. A non-empty from creates the branch at that start point.
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

// Worktrees lists the main worktree first.
func (s System) Worktrees(ctx context.Context, repo string) ([]Worktree, error) {
	out, err := s.run(ctx, repo, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return nil, err
	}
	return parseWorktrees(string(out)), nil
}

// parseWorktrees reads the -z format: one NUL per line, an empty line between entries.
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

// RemoveWorktree refuses a worktree with changes unless force is set.
func (s System) RemoveWorktree(ctx context.Context, repo, path string, force bool) error {
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	_, err := s.run(ctx, repo, append(args, "--", path)...)
	return err
}

func (s System) PruneWorktrees(ctx context.Context, repo string) error {
	_, err := s.run(ctx, repo, "worktree", "prune")
	return err
}

// RepairWorktree reconnects a moved worktree.
func (s System) RepairWorktree(ctx context.Context, repo, path string) error {
	_, err := s.run(ctx, repo, "worktree", "repair", "--", path)

	if gerr, ok := errors.AsType[*Error](err); ok && strings.Contains(gerr.Stderr, "usage: git worktree") {
		return ErrRepairUnsupported // old git prints the usage for an unknown subcommand
	}

	return err
}
