package space

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/0mega24/tandem/internal/gen"
	"github.com/0mega24/tandem/internal/git"
	"github.com/0mega24/tandem/internal/state"
)

// loadHealed loads a space and makes its state agree with the disk and git (see heal). It saves
// the state when heal changed it.
func (m *Manager) loadHealed(ctx context.Context, name string) (state.Space, error) {
	sp, err := m.store.Load(name)
	if err != nil {
		return sp, err
	}
	return m.healSaved(ctx, sp)
}

func (m *Manager) healSaved(ctx context.Context, sp state.Space) (state.Space, error) {
	healed, changed, err := m.heal(ctx, sp)
	if err != nil || !changed {
		return healed, err
	}
	return healed, m.store.Save(healed)
}

// heal checks each worktree of the space. The state is a hint: the user can delete or move a
// worktree with other tools.
//
//   - Directory there, git knows it: nothing to do.
//   - Directory gone: git prunes its entry, and the repo leaves the space. Uncommitted work is
//     lost; commits stay on the branch.
//   - Directory there, git does not know it (the main clone or the worktree moved and came
//     back): git worktree repair connects them again.
//
// A main clone that git cannot read is left as it is: status shows the error for that repo.
// Then healGenerated checks the generated files.
func (m *Manager) heal(ctx context.Context, sp state.Space) (state.Space, bool, error) {
	var keep []state.RepoEntry
	sp, changed := m.healGenerated(sp)

	for _, e := range sp.Repos {
		if e.Symlink {
			keep = append(keep, e)
			continue
		}

		trees, err := m.git.Worktrees(ctx, e.RepoPath)
		if err != nil {
			keep = append(keep, e)
			continue
		}
		listed := slices.ContainsFunc(trees, func(t git.Worktree) bool { return resolve(t.Path) == resolve(e.WorktreePath) })
		_, statErr := os.Stat(e.WorktreePath)
		exists := statErr == nil

		switch {
		case exists && listed:
			keep = append(keep, e)

		case !exists:
			if listed {
				if err := m.git.PruneWorktrees(ctx, e.RepoPath); err != nil {
					return sp, false, fmt.Errorf("%s: %w", e.Name, err)
				}
			}
			m.warn("%s: worktree %s is gone, so %s left space %s; its commits are still on branch %s",
				e.Name, e.WorktreePath, e.Name, sp.Name, sp.Branch)
			changed = true

		default:
			err := m.git.RepairWorktree(ctx, e.RepoPath, e.WorktreePath)
			if errors.Is(err, git.ErrRepairUnsupported) {
				return sp, false, fmt.Errorf("%s: git does not know worktree %s and is too old to repair it; "+
					"move the directory away and run tdm add %s %s again: %w", e.Name, e.WorktreePath, sp.Name, e.Name, err)
			}
			if err != nil {
				return sp, false, fmt.Errorf("%s: repair worktree: %w", e.Name, err)
			}
			m.warn("%s: repaired worktree %s", e.Name, e.WorktreePath)
			keep = append(keep, e)
		}
	}

	sp.Repos = keep
	return sp, changed, nil
}

// healGenerated makes the generated list agree with the disk. A wtg state file with
// go_workspace gets a go.work entry with the hash of the file on disk, and the default generator
// set. The flag goes also when go.work is gone: the set is all that is left to keep. A generated
// file that is gone leaves the list.
func (m *Manager) healGenerated(sp state.Space) (state.Space, bool) {
	changed := false
	if sp.GoWorkspace && len(sp.Generated) == 0 {
		b, err := os.ReadFile(filepath.Join(sp.Path, "go.work"))
		if err == nil {
			sp.Generated = []state.GeneratedFile{{Generator: "go", Path: "go.work", SHA256: gen.Hash(b), Stale: []string{"go.work.sum"}}}
		}
		defaults, _ := gen.Select(nil) // nil never fails
		sp.Generators = recordSet(defaults)
		sp.GoWorkspace = false
		changed = true
	}

	var keep []state.GeneratedFile
	for _, f := range sp.Generated {
		if _, err := os.Lstat(filepath.Join(sp.Path, filepath.FromSlash(f.Path))); errors.Is(err, fs.ErrNotExist) {
			m.warn("%s: generated file %s is gone, so it left the state of space %s", f.Generator, f.Path, sp.Name)
			changed = true
			continue
		}
		keep = append(keep, f)
	}
	sp.Generated = keep
	return sp, changed
}
