package space

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/0mega24/tandem/internal/state"
)

// ErrDeclined means the user said no to removing work that is not pushed.
var ErrDeclined = errors.New("nothing removed")

// Risk is work that a removal would lose or leave behind in one worktree.
type Risk struct {
	Name     string
	Changed  int // uncommitted paths, untracked included
	Unpushed int // commits on the space branch that origin does not have
}

// RemoveOptions are the choices of RemoveRepos.
type RemoveOptions struct {
	// DeleteBranch deletes the space branch in each repo when it is merged (git branch -d).
	DeleteBranch bool

	// Confirm is asked when a worktree has changes or unpushed commits. Nil means no.
	Confirm func([]Risk) bool
}

// Removed is what RemoveRepos did with one repo.
type Removed struct {
	Entry     state.RepoEntry
	Relinked  bool  // an always.repos repo: its worktree became the symlink again
	BranchErr error // set when DeleteBranch could not delete the branch (it stays)
}

// RemoveRepos takes repos out of a space. A worktree with changes or unpushed commits is
// removed only when Confirm says yes. A repo that is in always.repos gets its symlink back; a
// repo that is a symlink is dropped from the space.
//
// It is not a saga: a removed worktree cannot be put back as it was. It stops at the first
// failure and saves the state with the repos that it removed. Then the generators write their
// files again for the members that are left, like tdm generate.
func (m *Manager) RemoveRepos(ctx context.Context, name string, names []string, opts RemoveOptions) ([]Removed, GenerateResult, error) {
	out, err := m.removeRepos(ctx, name, names, opts)
	if err != nil || len(out) == 0 {
		return out, GenerateResult{}, err
	}

	generated, err := m.Generate(ctx, name, GenerateOptions{})
	if err != nil {
		return out, generated, fmt.Errorf("the repos are removed, but the workspace files are not up to date; run tdm generate %s: %w", name, err)
	}

	after, err := m.store.Load(name)
	if err != nil {
		return out, generated, err
	}
	targets := make([]state.RepoEntry, len(out))
	for i, r := range out {
		targets[i] = r.Entry
	}
	m.runHook(ctx, after, EventRemove, after.Repos, targets)
	return out, generated, nil
}

func (m *Manager) removeRepos(ctx context.Context, name string, names []string, opts RemoveOptions) ([]Removed, error) {
	if len(names) == 0 {
		return nil, errors.New("give at least one repo")
	}

	sp, err := m.loadHealed(ctx, name)
	if err != nil {
		return nil, err
	}
	idx, err := memberIndexes(sp, names)
	if err != nil {
		return nil, err
	}

	targets := make([]state.RepoEntry, len(idx))
	for i, j := range idx {
		targets[i] = sp.Repos[j]
	}
	risks, err := m.risks(ctx, targets)
	if err != nil {
		return nil, err
	}
	force := len(risks) > 0
	if force && (opts.Confirm == nil || !opts.Confirm(risks)) {
		return nil, ErrDeclined
	}

	always, err := m.alwaysRepoNames()
	if err != nil {
		return nil, err
	}

	var out []Removed
	for _, e := range targets {
		how := removeHow{force: force, deleteBranch: opts.DeleteBranch, relink: slices.Contains(always, e.Name)}
		r, err := m.removeOne(ctx, sp, e, how)
		if err != nil {
			return out, errors.Join(fmt.Errorf("%s: %w", e.Name, err), m.saveRemoved(sp, out))
		}
		out = append(out, r)
	}
	return out, m.saveRemoved(sp, out)
}

// memberIndexes resolves names against the repos of the space, not the repos on disk: a repo
// that was renamed or moved under the root can still be removed.
func memberIndexes(sp state.Space, names []string) ([]int, error) {
	repos := make([]Repo, len(sp.Repos))
	for i, e := range sp.Repos {
		repos[i] = Repo{Name: e.Name, Path: e.RepoPath}
	}
	picked, err := ResolveAll(repos, names)
	if err != nil {
		return nil, fmt.Errorf("space %s: %w", sp.Name, err)
	}

	idx := make([]int, len(picked))
	for i, r := range picked {
		idx[i] = slices.IndexFunc(sp.Repos, func(e state.RepoEntry) bool { return e.Name == r.Name })
	}
	return idx, nil
}

// risks returns the worktrees that have changes or unpushed commits. Symlinks have none: they
// are the main clone, which a removal does not touch.
func (m *Manager) risks(ctx context.Context, entries []state.RepoEntry) ([]Risk, error) {
	var out []Risk
	for _, e := range entries {
		if e.Symlink {
			continue
		}
		ms := m.memberStatus(ctx, e)
		if ms.Err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name, ms.Err)
		}
		if !ms.Status.Clean() || ms.Unpushed > 0 {
			out = append(out, Risk{Name: e.Name, Changed: len(ms.Status.Changed), Unpushed: ms.Unpushed})
		}
	}
	return out, nil
}

// alwaysRepoNames returns the short names of the always.repos that exist.
func (m *Manager) alwaysRepoNames() ([]string, error) {
	if len(m.cfg.Always.Repos) == 0 {
		return nil, nil
	}
	all, err := m.Repos()
	if err != nil {
		return nil, err
	}
	repos, err := ResolveAll(all, m.cfg.Always.Repos)
	if err != nil {
		return nil, fmt.Errorf("always.repos: %w", err)
	}

	names := make([]string, len(repos))
	for i, r := range repos {
		names[i] = r.Name
	}
	return names, nil
}

// removeHow says how removeOne treats one repo.
type removeHow struct {
	force        bool // remove a worktree with changes (the user said yes)
	deleteBranch bool
	forceBranch  bool // delete the branch when it is not merged too
	relink       bool // put the always.repos symlink back
}

func (m *Manager) removeOne(ctx context.Context, sp state.Space, e state.RepoEntry, how removeHow) (Removed, error) {
	r := Removed{Entry: e}

	if e.Symlink {
		if err := os.Remove(e.WorktreePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return r, err
		}
		removeEmptyParents(filepath.Dir(e.WorktreePath), sp.Path)
		return r, nil
	}

	if err := m.git.RemoveWorktree(ctx, e.RepoPath, e.WorktreePath, how.force); err != nil {
		return r, err
	}
	if how.deleteBranch {
		r.BranchErr = m.git.DeleteBranch(ctx, e.RepoPath, sp.Branch, how.forceBranch)
	}

	if how.relink {
		r.Relinked = true
		return r, os.Symlink(e.RepoPath, e.WorktreePath)
	}
	removeEmptyParents(filepath.Dir(e.WorktreePath), sp.Path)
	return r, nil
}

// saveRemoved saves the space without the removed repos, and with the symlinks that came back.
func (m *Manager) saveRemoved(sp state.Space, removed []Removed) error {
	if len(removed) == 0 {
		return nil
	}

	var repos []state.RepoEntry
	for _, e := range sp.Repos {
		i := slices.IndexFunc(removed, func(r Removed) bool { return r.Entry.Name == e.Name })
		switch {
		case i < 0:
			repos = append(repos, e)
		case removed[i].Relinked:
			e.Symlink = true
			repos = append(repos, e)
		}
	}

	// The links that a generator made inside a removed worktree went with it (generators spec,
	// Meson rule 7), so they leave the state now, not with a heal warning later.
	var files []state.GeneratedFile
	for _, f := range sp.Generated {
		p := filepath.Join(sp.Path, filepath.FromSlash(f.Path))
		gone := f.Target != "" && slices.ContainsFunc(removed, func(r Removed) bool {
			in, err := filepath.Rel(r.Entry.WorktreePath, p)
			return err == nil && filepath.IsLocal(in)
		})
		if !gone {
			files = append(files, f)
		}
	}

	sp.Repos, sp.Generated = repos, files
	return m.store.Save(sp)
}
