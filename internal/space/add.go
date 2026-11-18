package space

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/0mega24/tandem/internal/gen"
	"github.com/0mega24/tandem/internal/saga"
	"github.com/0mega24/tandem/internal/state"
)

// AddRepos puts a worktree on the space branch in each named repo. A repo that is an
// always.repos symlink in the space gets a worktree in place of the symlink. Like NewSpace, all
// checks run first and a failed step undoes the steps before it.
func (m *Manager) AddRepos(ctx context.Context, name string, names []string) (NewResult, error) {
	if len(names) == 0 {
		return NewResult{}, errors.New("give at least one repo")
	}

	sp, err := m.loadHealed(ctx, name)
	if err != nil {
		return NewResult{}, err
	}
	all, err := m.Repos()
	if err != nil {
		return NewResult{}, err
	}
	repos, err := ResolveAll(all, names)
	if err != nil {
		return NewResult{}, err
	}
	ids, err := gen.Select(sp.Generators)
	if err != nil {
		return NewResult{}, fmt.Errorf("space %s: %w", sp.Name, err)
	}

	next := sp
	next.Repos = slices.Clone(sp.Repos)
	var steps []saga.Step
	var added []NewRepo
	var errs []error

	for _, r := range repos {
		i := slices.IndexFunc(sp.Repos, func(e state.RepoEntry) bool { return e.Name == r.Name })
		if i >= 0 && !sp.Repos[i].Symlink {
			errs = append(errs, fmt.Errorf("%s is in space %s", r.Name, sp.Name))
			continue
		}

		p, err := m.planWorktree(ctx, r, sp)
		if err != nil {
			errs = append(errs, err)
			continue
		}

		if i >= 0 {
			if err := checkLink(sp.Repos[i]); err != nil {
				errs = append(errs, err)
				continue
			}
			steps = append(steps, unlinkStep(sp.Repos[i]))
			next.Repos[i] = p.entry
		} else {
			if _, err := os.Lstat(p.entry.WorktreePath); err == nil {
				errs = append(errs, fmt.Errorf("%s: %s exists", r.Name, p.entry.WorktreePath))
				continue
			}
			next.Repos = append(next.Repos, p.entry)
		}

		steps = append(steps, m.worktreeStep(sp, p))
		if len(p.secrets) > 0 {
			steps = append(steps, secretsStep(p))
		}
		added = append(added, NewRepo{
			Entry:   p.entry,
			From:    strings.TrimPrefix(p.from, "refs/heads/"),
			Secrets: p.secrets,
			WasLink: i >= 0,
		})
	}
	if err := errors.Join(errs...); err != nil {
		return NewResult{}, err
	}

	var generated GenerateResult
	steps = append(steps, m.generateStep(&next, ids, &generated))
	steps = append(steps, saga.Step{
		Name: "save state",
		Do:   func(context.Context) error { return m.store.Save(next) },
		Undo: func(context.Context) error { return m.store.Save(sp) },
	})
	if err := saga.Run(ctx, steps); err != nil {
		return NewResult{}, err
	}

	changed := make([]state.RepoEntry, len(added))
	for i, a := range added {
		changed[i] = a.Entry
	}
	m.runHook(ctx, next, EventAdd, next.Repos, changed)

	return NewResult{Space: next, Repos: added, Generated: generated}, nil
}

// checkLink makes sure that the path of a symlink entry is still the symlink, so that the
// unlink step removes nothing else.
func checkLink(e state.RepoEntry) error {
	fi, err := os.Lstat(e.WorktreePath)
	if err != nil {
		return fmt.Errorf("%s: %w", e.Name, err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("%s: %s is not the always.repos symlink", e.Name, e.WorktreePath)
	}
	return nil
}

func unlinkStep(e state.RepoEntry) saga.Step {
	return saga.Step{
		Name: "unlink " + e.Name,
		Do:   func(context.Context) error { return os.Remove(e.WorktreePath) },
		Undo: func(context.Context) error {
			// The worktree undo before this one may have removed an empty org directory.
			if err := os.MkdirAll(filepath.Dir(e.WorktreePath), 0o755); err != nil { //nolint:gosec // like the space root
				return err
			}
			return os.Symlink(e.RepoPath, e.WorktreePath)
		},
	}
}
