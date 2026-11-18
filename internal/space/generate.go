package space

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/0mega24/tandem/internal/gen"
	"github.com/0mega24/tandem/internal/saga"
	"github.com/0mega24/tandem/internal/state"
)

// GenerateOptions are the choices of Generate.
type GenerateOptions struct {
	DryRun bool // decide and report, but write nothing
	Force  bool // replace files that are not tdm's or that a person changed, and keep a copy
}

// GeneratorRun is what one generator found and planned.
type GeneratorRun struct {
	ID       string
	Members  []string // short names of the members that it detected
	Warnings []string
}

// GenerateResult is the result of Generate.
type GenerateResult struct {
	Space    state.Space // the state after the run
	Runs     []GeneratorRun
	Changes  []gen.Change
	Warnings []string // owned links that left the state, and deletes that failed
}

// Generate runs the generators of the space on its current members and writes their files.
// The ownership check of gen.Prepare decides each file. The writes and the state save are one
// saga, so a failure puts the old files back. A dry run heals the state in memory only.
func (m *Manager) Generate(ctx context.Context, name string, opts GenerateOptions) (GenerateResult, error) {
	sp, err := m.store.Load(name)
	if err != nil {
		return GenerateResult{}, err
	}
	if opts.DryRun {
		sp, _, err = m.heal(ctx, sp)
	} else {
		sp, err = m.healSaved(ctx, sp)
	}
	if err != nil {
		return GenerateResult{}, err
	}

	ids, err := gen.Select(sp.Generators)
	if err != nil {
		return GenerateResult{}, fmt.Errorf("space %s: %w", sp.Name, err)
	}
	runs, plans, err := m.plan(ctx, sp, ids)
	if err != nil {
		return GenerateResult{}, err
	}

	a, err := gen.Prepare(sp.Path, owned(sp.Generated), plans, memberDirs(sp), opts.Force, m.ignored(ctx, sp))
	if err != nil {
		return GenerateResult{}, fmt.Errorf("space %s: %w", sp.Name, err)
	}

	next := sp
	next.Generators = recordSet(ids) // a space without a set (made before generators) keeps today's defaults
	next.Generated = generated(a.Owned)
	res := GenerateResult{Space: next, Runs: runs, Changes: a.Changes}
	if opts.DryRun {
		res.Space, res.Warnings = sp, a.Warnings()
		return res, nil
	}

	steps := append(a.Steps(), saga.Step{
		Name: "save state",
		Do:   func(context.Context) error { return m.store.Save(next) },
		Undo: func(context.Context) error { return m.store.Save(sp) },
	})
	if err := saga.Run(ctx, steps); err != nil {
		return GenerateResult{}, err
	}
	res.Warnings = a.Warnings()
	return res, nil
}

// generateStep writes the files of the generators ids for *sp, as one step of the new and add
// sagas: the worktrees that it reads exist only after the steps before it. Do puts the set and
// the owned files in *sp, for the save step after it, and the result in *out. Files that a
// person changed or that tdm did not write are skipped: only tdm generate --force replaces them.
func (m *Manager) generateStep(sp *state.Space, ids []string, out *GenerateResult) saga.Step {
	var done []saga.Step
	return saga.Step{
		Name: "generate files",
		Do: func(ctx context.Context) error {
			runs, plans, err := m.plan(ctx, *sp, ids)
			if err != nil {
				return err
			}
			a, err := gen.Prepare(sp.Path, owned(sp.Generated), plans, memberDirs(*sp), false, m.ignored(ctx, *sp))
			if err != nil {
				return err
			}
			steps := a.Steps()
			if err := saga.Run(ctx, steps); err != nil {
				return err // Run undid its own steps
			}

			done = steps
			sp.Generators = recordSet(ids)
			sp.Generated = generated(a.Owned)
			*out = GenerateResult{Runs: runs, Changes: a.Changes, Warnings: a.Warnings()}
			return nil
		},
		Undo: func(ctx context.Context) error {
			var errs []error
			for _, s := range slices.Backward(done) {
				if s.Undo != nil {
					errs = append(errs, s.Undo(ctx))
				}
			}
			return errors.Join(errs...)
		},
	}
}

// plan runs Detect and Plan of each generator in ids on the members of the space.
func (m *Manager) plan(ctx context.Context, sp state.Space, ids []string) ([]GeneratorRun, map[string]gen.Plan, error) {
	if len(ids) == 0 {
		return nil, map[string]gen.Plan{}, nil // no git calls for a space without generators
	}
	members, err := m.members(ctx, sp)
	if err != nil {
		return nil, nil, err
	}

	var runs []GeneratorRun
	plans := map[string]gen.Plan{}
	for _, id := range ids {
		g, _ := gen.ByID(id) // Select checked the IDs
		table := m.cfg.Generate.Table(id)
		opts := gen.DetectOptions{Depth: m.cfg.Generate.DetectDepth, Config: table}

		run := GeneratorRun{ID: id}
		var found []gen.Detection
		for _, mb := range members {
			d, err := g.Detect(mb, opts)
			if err != nil {
				return nil, nil, fmt.Errorf("%s: detect in %s: %w", id, mb.Name, err)
			}
			if d != nil {
				found = append(found, *d)
				run.Members = append(run.Members, mb.Name)
			}
		}

		p, err := g.Plan(gen.Space{Name: sp.Name, Root: sp.Path, Branch: sp.Branch, Members: members, Config: table, Symlinks: symlinks(sp)}, found)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: plan: %w", id, err)
		}
		run.Warnings = p.Warnings
		runs = append(runs, run)
		plans[id] = p
	}
	return runs, plans, nil
}

// members returns the worktrees of the space as generator members, sorted by name. Symlinks are
// the main clones, which a generator must not see.
func (m *Manager) members(ctx context.Context, sp state.Space) ([]gen.Member, error) {
	var out []gen.Member
	for _, e := range sp.Repos {
		if e.Symlink {
			continue
		}
		origin, err := m.git.OriginURL(ctx, e.RepoPath)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name, err)
		}
		out = append(out, gen.Member{Name: e.Name, WorktreePath: e.WorktreePath, RepoPath: e.RepoPath, OriginURL: origin})
	}
	slices.SortFunc(out, func(a, b gen.Member) int { return strings.Compare(a.Name, b.Name) })
	return out, nil
}

// symlinks returns the always.repos symlinks of the space, sorted by name.
func symlinks(sp state.Space) []gen.Member {
	var out []gen.Member
	for _, e := range sp.Repos {
		if e.Symlink {
			out = append(out, gen.Member{Name: e.Name, WorktreePath: e.WorktreePath, RepoPath: e.RepoPath})
		}
	}
	slices.SortFunc(out, func(a, b gen.Member) int { return strings.Compare(a.Name, b.Name) })
	return out
}

// memberDirs returns the path of each repo entry relative to the space root, for gen.Prepare.
// Symlinks are in it too: no plan may write through one.
func memberDirs(sp state.Space) []string {
	var out []string
	for _, e := range sp.Repos {
		if rel, err := filepath.Rel(sp.Path, e.WorktreePath); err == nil {
			out = append(out, filepath.ToSlash(rel))
		}
	}
	return out
}

// ignored returns the git check-ignore test for gen.Prepare: a link path relative to the space
// root is checked in the worktree of the member that holds it.
func (m *Manager) ignored(ctx context.Context, sp state.Space) func(string) (bool, error) {
	return func(rel string) (bool, error) {
		p := filepath.Join(sp.Path, filepath.FromSlash(rel))
		for _, e := range sp.Repos {
			if e.Symlink {
				continue
			}
			if in, err := filepath.Rel(e.WorktreePath, p); err == nil && filepath.IsLocal(in) {
				return m.git.CheckIgnore(ctx, e.WorktreePath, filepath.ToSlash(in))
			}
		}
		return false, nil
	}
}

// recordSet is the generator set as the state keeps it. An empty set is ["none"], because the
// state file omits an empty list and a missing list means the default set.
func recordSet(ids []string) []string {
	if len(ids) == 0 {
		return []string{"none"}
	}
	return ids
}

func owned(files []state.GeneratedFile) []gen.Owned {
	out := make([]gen.Owned, len(files))
	for i, f := range files {
		out[i] = gen.Owned{Generator: f.Generator, Path: f.Path, SHA256: f.SHA256, Target: f.Target, Key: f.Key, Stale: f.Stale}
	}
	return out
}

func generated(files []gen.Owned) []state.GeneratedFile {
	out := make([]state.GeneratedFile, len(files))
	for i, f := range files {
		out[i] = state.GeneratedFile{Generator: f.Generator, Path: f.Path, SHA256: f.SHA256, Target: f.Target, Key: f.Key, Stale: f.Stale}
	}
	return out
}
