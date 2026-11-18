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

// NewResult is what NewSpace made.
type NewResult struct {
	Space state.Space
	Repos []NewRepo // the worktrees, then the always.repos symlinks
	Files []string  // the always.files copies in the space root
}

// NewRepo is one repo that NewSpace or AddRepos put in the space.
type NewRepo struct {
	Entry   state.RepoEntry
	From    string   // the branch that a new branch started from; "" when the branch existed
	Secrets []string // the always.secrets copied into the worktree
	WasLink bool     // the worktree took the place of an always.repos symlink
}

// planned is one worktree that pre-flight checked.
type planned struct {
	entry   state.RepoEntry
	from    string   // start point of a new branch, as a full ref; "" when the branch exists
	secrets []string // the always.secrets that the main clone has
}

// newPlan is everything that pre-flight checked, for the saga to make.
type newPlan struct {
	space     state.Space
	worktrees []planned
	links     []state.RepoEntry
}

// NewSpace makes a space: one worktree on one branch in each repo, the always.* links and
// copies, and the state file. All checks run before the first change; when a later step fails,
// the finished steps are undone.
func (m *Manager) NewSpace(ctx context.Context, opts NewOptions) (NewResult, error) {
	plan, err := m.preflight(ctx, opts)
	if err != nil {
		return NewResult{}, err
	}
	sp := plan.space

	steps := []saga.Step{rootStep(sp.Path)}
	for _, p := range plan.worktrees {
		steps = append(steps, m.worktreeStep(sp, p))
		if len(p.secrets) > 0 {
			steps = append(steps, secretsStep(p))
		}
	}
	for _, e := range plan.links {
		steps = append(steps, linkStep(sp.Path, e))
	}
	for _, f := range m.cfg.Always.Files {
		steps = append(steps, fileStep(sp.Path, f))
	}
	steps = append(steps, saga.Step{
		Name: "save state",
		Do:   func(context.Context) error { return m.store.Create(sp) },
		Undo: func(context.Context) error { return m.store.Delete(sp.Name) },
	})

	if err := saga.Run(ctx, steps); err != nil {
		return NewResult{}, err
	}
	m.runHook(ctx, sp, EventCreate, sp.Repos, sp.Repos)

	res := NewResult{Space: sp}
	for _, p := range plan.worktrees {
		res.Repos = append(res.Repos, NewRepo{
			Entry:   p.entry,
			From:    strings.TrimPrefix(p.from, "refs/heads/"),
			Secrets: p.secrets,
		})
	}
	for _, e := range plan.links {
		res.Repos = append(res.Repos, NewRepo{Entry: e})
	}
	for _, f := range m.cfg.Always.Files {
		res.Files = append(res.Files, filepath.Base(f))
	}
	return res, nil
}

func (m *Manager) preflight(ctx context.Context, opts NewOptions) (newPlan, error) {
	sp, worktrees, all, err := m.preflightWorktrees(ctx, opts)
	if err != nil {
		return newPlan{}, err
	}

	named := make([]Repo, len(worktrees))
	for i, p := range worktrees {
		named[i] = Repo{Name: p.entry.Name, Path: p.entry.RepoPath}
	}
	links, err := m.alwaysLinks(all, named, sp.Path)
	if err != nil {
		return newPlan{}, err
	}
	sp.Repos = append(sp.Repos, links...)

	if err := checkFiles(m.cfg.Always.Files, sp.Repos); err != nil {
		return newPlan{}, err
	}

	return newPlan{space: sp, worktrees: worktrees, links: links}, nil
}

// preflightWorktrees checks the name, the space directory and the branch in each repo. It also
// returns all repos under the root, for the always.repos names.
func (m *Manager) preflightWorktrees(ctx context.Context, opts NewOptions) (state.Space, []planned, []Repo, error) {
	if err := state.ValidateName(opts.Name); err != nil {
		return state.Space{}, nil, nil, err
	}
	if len(opts.Repos) == 0 {
		return state.Space{}, nil, nil, errors.New("give at least one repo")
	}

	if _, err := m.store.Load(opts.Name); err == nil {
		return state.Space{}, nil, nil, fmt.Errorf("space %q exists", opts.Name)
	} else if !errors.Is(err, state.ErrNotFound) {
		return state.Space{}, nil, nil, err
	}

	all, err := m.Repos()
	if err != nil {
		return state.Space{}, nil, nil, err
	}
	repos, err := ResolveAll(all, opts.Repos)
	if err != nil {
		return state.Space{}, nil, nil, err
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
		return state.Space{}, nil, nil, err
	}

	// The worktrees go inside the root, so an empty root also means that no worktree path exists.
	if err := checkEmptyDir(sp.Path); err != nil {
		return state.Space{}, nil, nil, err
	}
	if err := m.checkNotNested(sp.Path, all); err != nil {
		return state.Space{}, nil, nil, err
	}

	// Without this check a bad name fails only in the first worktree step, after a change.
	if err := m.git.CheckBranchName(ctx, repos[0].Path, sp.Branch); err != nil {
		return state.Space{}, nil, nil, err
	}

	plan := make([]planned, len(repos))
	var errs []error
	for i, r := range repos {
		if plan[i], err = m.planWorktree(ctx, r, sp); err != nil {
			errs = append(errs, err)
		}
		sp.Repos = append(sp.Repos, plan[i].entry)
	}

	return sp, plan, all, errors.Join(errs...)
}

// planWorktree checks one repo for a worktree in sp: the branch rules and the secrets.
func (m *Manager) planWorktree(ctx context.Context, r Repo, sp state.Space) (planned, error) {
	p := planned{entry: state.RepoEntry{
		Name:         r.Name,
		RepoPath:     r.Path,
		WorktreePath: filepath.Join(sp.Path, filepath.FromSlash(r.Name)),
	}}

	var err error
	if p.from, err = m.branchStart(ctx, r, sp.Branch); err != nil {
		return p, fmt.Errorf("%s: %w", r.Name, err)
	}
	if p.secrets, err = secretsOf(r.Path, m.cfg.Always.Secrets); err != nil {
		return p, fmt.Errorf("%s: %w", r.Name, err)
	}
	return p, nil
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
			if t.Branch == branch && t.Prunable {
				return "", fmt.Errorf("branch %s is held by worktree %s, whose directory is gone (git worktree prune frees it)", branch, t.Path)
			}
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

// checkNotNested refuses a space directory inside a main clone, where the worktrees would be
// files of that repo, or inside another space.
func (m *Manager) checkNotNested(dir string, repos []Repo) error {
	for _, r := range repos {
		if within(dir, r.Path) {
			return fmt.Errorf("space directory %s is inside repo %s", dir, r.Name)
		}
	}

	spaces, err := m.store.List()
	if err != nil {
		return err
	}
	for _, sp := range spaces {
		if within(dir, sp.Path) {
			return fmt.Errorf("space directory %s is inside space %s", dir, sp.Name)
		}
	}
	return nil
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

// rootStep makes the space directory and its missing parents. Undo removes what it made.
func rootStep(root string) saga.Step {
	top := "" // the highest directory that this step made
	return saga.Step{
		Name: "make " + root,
		Do: func(context.Context) error {
			top = firstMissing(root)
			if top == "" {
				return nil
			}
			return os.MkdirAll(root, 0o755) //nolint:gosec // a space holds source checkouts that other tools and users read
		},
		Undo: func(context.Context) error {
			if top == "" {
				return nil
			}
			if err := os.Remove(root); err != nil {
				return err
			}
			removeEmptyParents(filepath.Dir(root), filepath.Dir(top))
			return nil
		},
	}
}

// firstMissing returns the highest missing directory on the way to dir, or "" when dir exists.
func firstMissing(dir string) string {
	top := ""
	for p := dir; ; p = filepath.Dir(p) {
		if _, err := os.Stat(p); err == nil {
			return top
		}
		top = p
		if filepath.Dir(p) == p {
			return top
		}
	}
}

func (m *Manager) worktreeStep(sp state.Space, p planned) saga.Step {
	e := p.entry
	return saga.Step{
		Name: "worktree " + e.Name,
		Do: func(ctx context.Context) error {
			err := m.git.AddWorktree(ctx, e.RepoPath, e.WorktreePath, sp.Branch, p.from)
			if err != nil {
				// Ctrl-C or a crash of git can stop it after it made the branch or the
				// directory, and the saga undoes only finished steps.
				m.dropPartialWorktree(context.WithoutCancel(ctx), sp, p)
			}
			return err
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

// dropPartialWorktree removes what a failed AddWorktree may have left. Pre-flight saw no path
// and, when from is set, no branch, so all that is there now is from this step. Each part may
// be missing, so the errors are not useful and the step reports the error of AddWorktree.
func (m *Manager) dropPartialWorktree(ctx context.Context, sp state.Space, p planned) {
	e := p.entry
	_ = m.git.RemoveWorktree(ctx, e.RepoPath, e.WorktreePath, true)
	_ = os.RemoveAll(e.WorktreePath)
	_ = m.git.PruneWorktrees(ctx, e.RepoPath)
	if p.from != "" {
		_ = m.git.DeleteBranch(ctx, e.RepoPath, sp.Branch, true)
	}
	removeEmptyParents(filepath.Dir(e.WorktreePath), sp.Path)
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
