package space

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/0mega24/tandem/internal/saga"
	"github.com/0mega24/tandem/internal/state"
)

// NewOptions are the inputs of NewSpace. Branch and Path are optional.
type NewOptions struct {
	Name   string
	Repos  []string // names as the user gave them
	Branch string   // default: git.branch_prefix + Name
	Path   string   // default: spaces.root_dir/Name
}

// NewRepo is one repo that NewSpace put in the space.
type NewRepo struct {
	Entry     state.RepoEntry
	NewBranch bool // false when the branch existed and was checked out as it was
}

// planned is one worktree that pre-flight checked.
type planned struct {
	entry state.RepoEntry
	from  string // start point of a new branch; "" when the branch exists
}

// NewSpace makes a space: one worktree on one branch in each repo, and the state file. All checks
// run before the first change; when a later step fails, the finished steps are undone.
func (m *Manager) NewSpace(ctx context.Context, opts NewOptions) (state.Space, []NewRepo, error) {
	sp, plan, err := m.preflight(ctx, opts)
	if err != nil {
		return state.Space{}, nil, err
	}

	steps := []saga.Step{rootStep(sp.Path)}
	for _, p := range plan {
		steps = append(steps, m.worktreeStep(sp, p))
	}
	steps = append(steps, saga.Step{
		Name: "save state",
		Do:   func(context.Context) error { return m.store.Save(sp) },
		Undo: func(context.Context) error { return m.store.Delete(sp.Name) },
	})

	if err := saga.Run(ctx, steps); err != nil {
		return state.Space{}, nil, err
	}

	out := make([]NewRepo, len(plan))
	for i, p := range plan {
		out[i] = NewRepo{Entry: p.entry, NewBranch: p.from != ""}
	}
	return sp, out, nil
}

func (m *Manager) preflight(ctx context.Context, opts NewOptions) (state.Space, []planned, error) {
	if err := state.ValidateName(opts.Name); err != nil {
		return state.Space{}, nil, err
	}
	if len(opts.Repos) == 0 {
		return state.Space{}, nil, errors.New("give at least one repo")
	}

	if _, err := m.store.Load(opts.Name); err == nil {
		return state.Space{}, nil, fmt.Errorf("space %q exists", opts.Name)
	} else if !errors.Is(err, state.ErrNotFound) {
		return state.Space{}, nil, err
	}

	repos, err := m.selectRepos(opts.Repos)
	if err != nil {
		return state.Space{}, nil, err
	}

	sp := state.Space{
		Name:      opts.Name,
		Path:      opts.Path,
		Branch:    opts.Branch,
		CreatedAt: time.Now().UTC().Truncate(time.Second),
	}
	if sp.Branch == "" {
		sp.Branch = m.cfg.Git.BranchPrefix + opts.Name
	}
	if sp.Path == "" {
		sp.Path = filepath.Join(m.cfg.Spaces.RootDir, opts.Name)
	}
	if sp.Path, err = filepath.Abs(sp.Path); err != nil {
		return state.Space{}, nil, err
	}

	// The worktrees go inside the root, so an empty root also means that no worktree path exists.
	if err := checkEmptyDir(sp.Path); err != nil {
		return state.Space{}, nil, err
	}

	plan := make([]planned, len(repos))
	var errs []error
	for i, r := range repos {
		entry := state.RepoEntry{
			Name:         r.Name,
			RepoPath:     r.Path,
			WorktreePath: filepath.Join(sp.Path, filepath.FromSlash(r.Name)),
		}
		from, err := m.branchStart(ctx, r, sp.Branch)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", r.Name, err))
		}
		plan[i] = planned{entry: entry, from: from}
		sp.Repos = append(sp.Repos, entry)
	}

	return sp, plan, errors.Join(errs...)
}

// branchStart applies the branch rules for one repo. It returns the start point for a new branch,
// or "" to check out the existing branch as it is (never reset: it may hold work).
func (m *Manager) branchStart(ctx context.Context, r Repo, branch string) (string, error) {
	exists, err := m.git.BranchExists(ctx, r.Path, branch)
	if err != nil {
		return "", err
	}

	if exists {
		trees, err := m.git.Worktrees(ctx, r.Path)
		if err != nil {
			return "", err
		}
		for _, t := range trees {
			if t.Branch == branch {
				return "", fmt.Errorf("branch %s is checked out in %s", branch, t.Path)
			}
		}
		return "", nil
	}

	def, err := m.git.DefaultBranch(ctx, r.Path)
	if err != nil {
		return "", fmt.Errorf("find the default branch: %w", err)
	}

	// The new branch starts from the local default branch, which tdm repo sync keeps current.
	ok, err := m.git.BranchExists(ctx, r.Path, def)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("default branch %s has no local branch", def)
	}
	return "refs/heads/" + def, nil
}

func checkEmptyDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("space directory: %w", err)
	}
	if len(entries) > 0 {
		return fmt.Errorf("space directory %s is not empty", dir)
	}
	return nil
}

// rootStep makes the space directory. Undo removes it only when this step made it.
func rootStep(root string) saga.Step {
	made := false
	return saga.Step{
		Name: "make " + root,
		Do: func(context.Context) error {
			if _, err := os.Stat(root); err == nil {
				return nil
			}
			made = true
			return os.MkdirAll(root, 0o755) //nolint:gosec // a space holds source checkouts that other tools and users read
		},
		Undo: func(context.Context) error {
			if !made {
				return nil
			}
			return os.Remove(root)
		},
	}
}

func (m *Manager) worktreeStep(sp state.Space, p planned) saga.Step {
	e := p.entry
	return saga.Step{
		Name: "worktree " + e.Name,
		Do: func(ctx context.Context) error {
			return m.git.AddWorktree(ctx, e.RepoPath, e.WorktreePath, sp.Branch, p.from)
		},
		Undo: func(ctx context.Context) error {
			// The worktree is new, so force loses no work; the same holds for a new branch.
			if err := m.git.RemoveWorktree(ctx, e.RepoPath, e.WorktreePath, true); err != nil {
				return err
			}
			if p.from != "" {
				if err := m.git.DeleteBranch(ctx, e.RepoPath, sp.Branch, true); err != nil {
					return err
				}
			}
			removeEmptyParents(filepath.Dir(e.WorktreePath), sp.Path)
			return nil
		},
	}
}

// removeEmptyParents removes dir and its empty parents up to stop (not included): the org
// directories that git made for nested repos such as myorg/api. It stops at the first directory
// that it cannot remove, which is one that another repo of the space is in.
func removeEmptyParents(dir, stop string) {
	for dir != stop && strings.HasPrefix(dir, stop+string(filepath.Separator)) {
		if err := os.Remove(dir); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return
		}
		dir = filepath.Dir(dir)
	}
}
