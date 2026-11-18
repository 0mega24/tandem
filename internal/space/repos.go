package space

import (
	"context"
	"errors"
	"fmt"

	"github.com/0mega24/tandem/internal/git"
)

// Repos lists the main clones under discovery.root_dir.
func (m *Manager) Repos() ([]Repo, error) {
	return Discover(m.cfg.Discovery.RootDir, m.cfg.Discovery.MaxDepth)
}

// selectRepos resolves names, or returns all repos when names is empty.
func (m *Manager) selectRepos(names []string) ([]Repo, error) {
	repos, err := m.Repos()
	if err != nil || len(names) == 0 {
		return repos, err
	}

	return ResolveAll(repos, names)
}

// RepoStatus is the state of one main clone.
type RepoStatus struct {
	Repo   Repo
	Origin string
	Status git.Status
	Err    error // the git error for this repo; the others still have a status
}

// RepoStatus reads the status of the named main clones, or of all of them.
func (m *Manager) RepoStatus(ctx context.Context, names []string) ([]RepoStatus, error) {
	repos, err := m.selectRepos(names)
	if err != nil {
		return nil, err
	}

	out := make([]RepoStatus, len(repos))
	parallel(len(repos), func(i int) {
		r := RepoStatus{Repo: repos[i]}

		r.Status, r.Err = m.git.Status(ctx, r.Repo.Path)
		if r.Err == nil {
			r.Origin, r.Err = m.git.OriginURL(ctx, r.Repo.Path)
		}

		out[i] = r
	})

	return out, nil
}

// SyncOutcome is what SyncRepos did with one repo.
type SyncOutcome int

const (
	SyncUpToDate SyncOutcome = iota
	SyncUpdated
	SyncSkippedDirty
	SyncSkippedCheckedOut
	SyncFailed
)

func (o SyncOutcome) String() string {
	switch o {
	case SyncUpToDate:
		return "up to date"
	case SyncUpdated:
		return "updated"
	case SyncSkippedDirty:
		return "skipped: local changes"
	case SyncSkippedCheckedOut:
		return "skipped" // Err names the worktree.
	case SyncFailed:
		return "failed"
	}
	return fmt.Sprintf("SyncOutcome(%d)", int(o))
}

// SyncResult is the result for one repo.
type SyncResult struct {
	Repo    Repo
	Branch  string // the default branch
	Outcome SyncOutcome
	Err     error // set when Outcome is SyncFailed
}

// SyncRepos fetches each named main clone (or all) and fast-forwards its default branch. A clone
// with local changes is skipped. Worktrees are not touched.
func (m *Manager) SyncRepos(ctx context.Context, names []string) ([]SyncResult, error) {
	repos, err := m.selectRepos(names)
	if err != nil {
		return nil, err
	}

	out := make([]SyncResult, len(repos))
	parallel(len(repos), func(i int) {
		out[i] = m.syncRepo(ctx, repos[i])
	})

	return out, nil
}

func (m *Manager) syncRepo(ctx context.Context, repo Repo) SyncResult {
	res := SyncResult{Repo: repo, Outcome: SyncFailed}

	st, err := m.git.Status(ctx, repo.Path)
	if err != nil {
		res.Err = err
		return res
	}
	if !st.Clean() {
		res.Outcome = SyncSkippedDirty
		return res
	}

	if res.Branch, err = m.git.DefaultBranch(ctx, repo.Path); err != nil {
		res.Err = err
		return res
	}
	if err := m.git.Fetch(ctx, repo.Path); err != nil {
		res.Err = err
		return res
	}

	moved, err := m.git.FastForward(ctx, repo.Path, res.Branch)
	switch {
	case errors.Is(err, git.ErrCheckedOutElsewhere):
		res.Outcome, res.Err = SyncSkippedCheckedOut, err
	case err != nil:
		res.Err = err
	case moved:
		res.Outcome = SyncUpdated
	default:
		res.Outcome = SyncUpToDate
	}

	return res
}
