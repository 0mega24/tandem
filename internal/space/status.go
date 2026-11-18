package space

import (
	"context"
	"path/filepath"
	"slices"
	"strings"

	"github.com/0mega24/tandem/internal/git"
	"github.com/0mega24/tandem/internal/state"
)

// Space returns the state of one space.
func (m *Manager) Space(name string) (state.Space, error) {
	return m.store.Load(name)
}

// SpaceNames lists the names of all spaces, for shell completion.
func (m *Manager) SpaceNames() ([]string, error) {
	spaces, err := m.store.List()
	names := make([]string, len(spaces))
	for i, sp := range spaces {
		names[i] = sp.Name
	}
	return names, err
}

// SpaceStatus is the status of one space and its repos.
type SpaceStatus struct {
	Space state.Space
	Repos []MemberStatus
}

// MemberStatus is the status of one repo in a space.
type MemberStatus struct {
	Entry    state.RepoEntry
	Status   git.Status // zero for a symlink
	Unpushed int        // commits on the space branch that origin does not have
	Err      error
}

// Status returns the named spaces, or all of them. The space that holds dir (the current
// directory) comes first, so the one that the user is in is at the top.
func (m *Manager) Status(ctx context.Context, names []string, dir string) ([]SpaceStatus, error) {
	spaces, err := m.loadSpaces(names)
	if err != nil {
		return nil, err
	}

	if i := slices.IndexFunc(spaces, func(sp state.Space) bool { return within(dir, sp.Path) }); i > 0 {
		sp := spaces[i]
		spaces = slices.Insert(slices.Delete(spaces, i, i+1), 0, sp)
	}

	out := make([]SpaceStatus, len(spaces))
	for i, sp := range spaces {
		out[i] = SpaceStatus{Space: sp, Repos: make([]MemberStatus, len(sp.Repos))}
	}

	// One flat list of repos, so a space with many repos and one with few use the same workers.
	type job struct{ s, r int }
	var jobs []job
	for s, sp := range spaces {
		for r := range sp.Repos {
			jobs = append(jobs, job{s, r})
		}
	}

	parallel(len(jobs), func(i int) {
		j := jobs[i]
		out[j.s].Repos[j.r] = m.memberStatus(ctx, spaces[j.s].Repos[j.r], spaces[j.s].Branch)
	})

	return out, nil
}

func (m *Manager) memberStatus(ctx context.Context, e state.RepoEntry, branch string) MemberStatus {
	ms := MemberStatus{Entry: e}
	if e.Symlink {
		return ms
	}

	ms.Status, ms.Err = m.git.Status(ctx, e.WorktreePath)
	if ms.Err == nil {
		ms.Unpushed, ms.Err = m.git.Unpushed(ctx, e.RepoPath, branch)
	}
	return ms
}

func (m *Manager) loadSpaces(names []string) ([]state.Space, error) {
	if len(names) == 0 {
		return m.store.List()
	}

	var spaces []state.Space
	for _, n := range names {
		sp, err := m.store.Load(n)
		if err != nil {
			return nil, err
		}
		if !slices.ContainsFunc(spaces, func(s state.Space) bool { return s.Name == sp.Name }) {
			spaces = append(spaces, sp)
		}
	}
	return spaces, nil
}

// within reports whether dir is root or inside it.
func within(dir, root string) bool {
	rel, err := filepath.Rel(root, dir)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
