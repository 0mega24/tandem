package space

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/0mega24/tandem/internal/gen"
	"github.com/0mega24/tandem/internal/saga"
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
	Space state.Space
	Repos []Removed
	Files []string // the always.files copies that it removed

	// KeptFiles are files in the space root with the base name of an always.files entry that are
	// not the same as the source now: a person wrote or changed them, so they stay.
	KeptFiles []KeptFile

	// Generated is the delete or skip of each generated file: a file that a person changed stays.
	Generated []gen.Change

	KeptDir bool // the space directory had other files, so it stays
}

// KeptFile is a file that DeleteSpace did not remove, and the file it differs from.
type KeptFile struct {
	Name   string // base name in the space root
	Source string // the always.files entry, expanded
}

// DeleteSpace removes all worktrees and symlinks of a space, the always.files copies, the space
// directory when nothing else is in it, and the state. Like RemoveRepos it asks first about work
// that is not pushed, and it stops at the first failure with the state saved for a retry.
func (m *Manager) DeleteSpace(ctx context.Context, name string, opts DeleteOptions) (Deleted, error) {
	unlock, err := m.store.Lock(name)
	if err != nil {
		return Deleted{}, err
	}
	defer unlock()
	sp, err := m.loadHealed(ctx, name)
	if err != nil {
		return Deleted{}, err
	}
	res := Deleted{Space: sp}

	risks, err := m.risks(ctx, sp.Repos)
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
	res.Generated = m.deleteGenerated(ctx, sp)

	// The copies are found by the base names in the config now. A file that is not the same as
	// its source now is not a copy that tdm can remove: a person wrote or changed it, or the
	// source changed. A copy that the user replaced with a directory stays too.
	for _, f := range m.cfg.Always.Files {
		p := filepath.Join(sp.Path, filepath.Base(f))
		if fi, err := os.Lstat(p); err == nil && fi.Mode().IsRegular() {
			if !sameContent(p, f) {
				res.KeptFiles = append(res.KeptFiles, KeptFile{Name: filepath.Base(f), Source: f})
				continue
			}
			if err := os.Remove(p); err != nil {
				return res, errors.Join(err, m.saveRemoved(sp, res.Repos))
			}
			res.Files = append(res.Files, filepath.Base(f))
		}
	}

	if err := os.Remove(sp.Path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		res.KeptDir = true
	}
	if err := m.store.Delete(sp.Name); err != nil {
		return res, err
	}

	m.runHook(ctx, sp, EventDelete, sp.Repos, sp.Repos)
	return res, nil
}

// sameContent reports whether files a and b hold the same bytes. A file that cannot be read is
// not the same, so the copy stays.
func sameContent(a, b string) bool {
	x, err := os.ReadFile(a) //nolint:gosec // G304: a file in the space root
	if err != nil {
		return false
	}
	y, err := os.ReadFile(b) //nolint:gosec // G304: the path comes from the user's config
	return err == nil && bytes.Equal(x, y)
}

// deleteGenerated deletes the generated files that tdm owns and that nobody changed, with their
// stale files. A failure is a warning, as the generator spec says for cleanup: the saga puts the
// files back, and the space directory stays.
func (m *Manager) deleteGenerated(ctx context.Context, sp state.Space) []gen.Change {
	if len(sp.Generated) == 0 {
		return nil
	}
	plans := map[string]gen.Plan{} // an empty plan for each owner: Prepare deletes its files
	for _, f := range sp.Generated {
		plans[f.Generator] = gen.Plan{}
	}

	a, err := gen.Prepare(sp.Path, owned(sp.Generated), plans, memberDirs(sp), false, nil) // nil: delete makes no links
	if err == nil {
		err = saga.Run(ctx, a.Steps())
	}
	if err != nil {
		m.warn("space %s: generated files are kept: %v", sp.Name, err)
		return nil
	}
	for _, w := range a.Warnings() {
		m.warn("space %s: %s", sp.Name, w)
	}
	return a.Changes
}
