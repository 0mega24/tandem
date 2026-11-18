package space

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/0mega24/tandem/internal/state"
)

// DeleteOptions are the choices of DeleteSpace.
type DeleteOptions struct {
	// DeleteBranch deletes the space branch in each repo when it is merged (git branch -d).
	DeleteBranch bool

	// ForceBranch deletes the space branch also when it is not merged (git branch -D).
	ForceBranch bool

	// Confirm is asked when a worktree has changes or unpushed commits. Nil means no.
	Confirm func([]Risk) bool
}

// Deleted is what DeleteSpace did.
type Deleted struct {
	Space   state.Space
	Repos   []Removed
	Files   []string // the always.files copies that it removed
	KeptDir bool     // the space directory had other files, so it stays
}

// DeleteSpace removes all worktrees and symlinks of a space, the always.files copies, the space
// directory when nothing else is in it, and the state. Like RemoveRepos it asks first about work
// that is not pushed, and it stops at the first failure with the state saved for a retry.
func (m *Manager) DeleteSpace(ctx context.Context, name string, opts DeleteOptions) (Deleted, error) {
	sp, err := m.store.Load(name)
	if err != nil {
		return Deleted{}, err
	}
	res := Deleted{Space: sp}

	risks, err := m.risks(ctx, sp, sp.Repos)
	if err != nil {
		return res, err
	}
	force := len(risks) > 0
	if force && (opts.Confirm == nil || !opts.Confirm(risks)) {
		return res, ErrDeclined
	}

	how := removeHow{
		force:        force,
		deleteBranch: opts.DeleteBranch || opts.ForceBranch,
		forceBranch:  opts.ForceBranch,
	}
	for _, e := range sp.Repos {
		r, err := m.removeOne(ctx, sp, e, how)
		if err != nil {
			return res, errors.Join(fmt.Errorf("%s: %w", e.Name, err), m.saveRemoved(sp, res.Repos))
		}
		res.Repos = append(res.Repos, r)
	}

	// The copies are found by the base names in the config now. A copy that the user replaced
	// with a directory is not a copy, so it stays.
	for _, f := range m.cfg.Always.Files {
		p := filepath.Join(sp.Path, filepath.Base(f))
		if fi, err := os.Lstat(p); err == nil && fi.Mode().IsRegular() {
			if err := os.Remove(p); err != nil {
				return res, errors.Join(err, m.saveRemoved(sp, res.Repos))
			}
			res.Files = append(res.Files, filepath.Base(f))
		}
	}

	if err := os.Remove(sp.Path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		res.KeptDir = true
	}
	return res, m.store.Delete(sp.Name)
}
