package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/0mega24/tandem/internal/gen"
	"github.com/0mega24/tandem/internal/space"
	"github.com/0mega24/tandem/internal/state"
	"github.com/0mega24/tandem/internal/ui"
)

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false) // diffs and errors hold < and >; keep them readable
	return enc.Encode(v)
}

type errorOutput struct {
	Error string `json:"error"`
}
type versionOutput struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
}
type pathOutput struct {
	Path string `json:"path"`
}
type configPathOutput struct {
	Path string `json:"path"`
}
type configInitOutput struct {
	Path string `json:"path"`
}
type configOutput struct {
	Layers   []configLayerOutput `json:"layers"`
	UserPath string              `json:"user_path"`
}
type configLayerOutput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}
type spaceOutput struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Branch string `json:"branch"`
}

func jsonSpace(sp state.Space) spaceOutput {
	return spaceOutput{Name: sp.Name, Path: sp.Path, Branch: sp.Branch}
}

type statusOutput struct {
	Spaces []spaceStatusOutput `json:"spaces"`
}
type spaceStatusOutput struct {
	spaceOutput
	Repos     []memberStatusOutput     `json:"repos"`
	Generated *[]generatedStatusOutput `json:"generated,omitempty"`
}
type memberStatusOutput struct {
	Name         string   `json:"name"`
	RepoPath     string   `json:"repo_path"`
	WorktreePath string   `json:"worktree_path"`
	Symlink      bool     `json:"symlink"`
	Branch       string   `json:"branch"`
	Changed      []string `json:"changed"`
	Ahead        int      `json:"ahead"`
	Behind       int      `json:"behind"`
	Error        string   `json:"error,omitempty"`
}
type generatedStatusOutput struct {
	Path      string `json:"path"`
	Generator string `json:"generator"`
	State     string `json:"state"`
	Error     string `json:"error,omitempty"`
}

func jsonStatus(list []space.SpaceStatus, long bool) (statusOutput, error) {
	out := statusOutput{Spaces: make([]spaceStatusOutput, len(list))}
	var failures []error
	for i, s := range list {
		row := spaceStatusOutput{spaceOutput: jsonSpace(s.Space), Repos: make([]memberStatusOutput, len(s.Repos))}
		for j, r := range s.Repos {
			row.Repos[j] = memberStatusOutput{
				Name:         r.Entry.Name,
				RepoPath:     r.Entry.RepoPath,
				WorktreePath: r.Entry.WorktreePath,
				Symlink:      r.Entry.Symlink,
				Branch:       r.Status.Branch,
				Changed:      append([]string{}, r.Status.Changed...),
				Ahead:        r.Unpushed,
				Behind:       r.Status.Behind,
			}
			if r.Err != nil {
				row.Repos[j].Error = r.Err.Error()
				failures = append(failures, fmt.Errorf("%s/%s: %w", s.Space.Name, r.Entry.Name, r.Err))
			}
		}
		if long {
			generated := make([]generatedStatusOutput, len(s.Generated))
			for j, g := range s.Generated {
				st := "intact"
				switch g.State {
				case gen.Changed:
					st = "changed"
				case gen.Gone:
					st = "gone"
				}
				generated[j] = generatedStatusOutput{Path: g.File.Path, Generator: g.File.Generator, State: st}
				if g.Err != nil {
					generated[j].Error = g.Err.Error()
				}
			}
			row.Generated = &generated
		}
		out.Spaces[i] = row
	}
	return out, errors.Join(failures...)
}

type repoStatusOutput struct {
	Repos []repoStatusRowOutput `json:"repos"`
}
type repoStatusRowOutput struct {
	Name    string   `json:"name"`
	Path    string   `json:"path"`
	Origin  string   `json:"origin"`
	Branch  string   `json:"branch"`
	Changed []string `json:"changed"`
	Ahead   int      `json:"ahead"`
	Behind  int      `json:"behind"`
	Error   string   `json:"error,omitempty"`
}

func jsonRepoStatus(list []space.RepoStatus) (repoStatusOutput, error) {
	out := repoStatusOutput{Repos: make([]repoStatusRowOutput, len(list))}
	var failures []error
	for i, r := range list {
		out.Repos[i] = repoStatusRowOutput{
			Name:    r.Repo.Name,
			Path:    r.Repo.Path,
			Origin:  r.Origin,
			Branch:  r.Status.Branch,
			Changed: append([]string{}, r.Status.Changed...),
			Ahead:   r.Status.Ahead,
			Behind:  r.Status.Behind,
		}
		if r.Err != nil {
			out.Repos[i].Error = r.Err.Error()
			failures = append(failures, fmt.Errorf("%s: %w", r.Repo.Name, r.Err))
		}
	}
	return out, errors.Join(failures...)
}

type repoSyncOutput struct {
	Repos []repoSyncRowOutput `json:"repos"`
}
type repoSyncRowOutput struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Branch  string `json:"branch"`
	Outcome string `json:"outcome"`
	Error   string `json:"error,omitempty"`
}

func jsonRepoSync(list []space.SyncResult) (repoSyncOutput, error) {
	out := repoSyncOutput{Repos: make([]repoSyncRowOutput, len(list))}
	var failures []error
	for i, r := range list {
		outcome := "up_to_date"
		switch r.Outcome {
		case space.SyncUpdated:
			outcome = "updated"
		case space.SyncSkippedDirty:
			outcome = "skipped_dirty"
		case space.SyncSkippedCheckedOut:
			outcome = "skipped_checked_out"
		case space.SyncFailed:
			outcome = "failed"
			failures = append(failures, fmt.Errorf("%s: %w", r.Repo.Name, r.Err))
		}
		out.Repos[i] = repoSyncRowOutput{Name: r.Repo.Name, Path: r.Repo.Path, Branch: r.Branch, Outcome: outcome}
		if r.Err != nil {
			out.Repos[i].Error = r.Err.Error()
		}
	}
	return out, errors.Join(failures...)
}

type generateOutput struct {
	Space    spaceOutput          `json:"space"`
	DryRun   bool                 `json:"dry_run"`
	Runs     []generatorRunOutput `json:"runs"`
	Changes  []changeOutput       `json:"changes"`
	Warnings []string             `json:"warnings"`
}
type generatorRunOutput struct {
	ID       string   `json:"id"`
	Members  []string `json:"members"`
	Warnings []string `json:"warnings"`
}
type changeOutput struct {
	Generator string  `json:"generator"`
	Path      string  `json:"path"`
	Action    string  `json:"action"`
	Reason    string  `json:"reason"`
	Target    string  `json:"target,omitempty"`
	Diff      *string `json:"diff,omitempty"`
}

func jsonChanges(changes []gen.Change, dryRun bool) []changeOutput {
	out := make([]changeOutput, len(changes))
	for i, c := range changes {
		out[i] = changeOutput{Generator: c.Generator, Path: c.Path, Action: c.Action.String(), Reason: c.Reason, Target: c.Target}
		if dryRun {
			diff := ""
			switch c.Action {
			case gen.Write:
				diff = ui.Diff(c.Path, c.Old, c.New)
			case gen.Delete:
				diff = ui.Diff(c.Path, c.Old, nil)
			}
			out[i].Diff = &diff
		}
	}
	return out
}

func jsonGenerate(res space.GenerateResult, dryRun bool) generateOutput {
	out := generateOutput{
		Space:    jsonSpace(res.Space),
		DryRun:   dryRun,
		Runs:     make([]generatorRunOutput, len(res.Runs)),
		Changes:  jsonChanges(res.Changes, dryRun),
		Warnings: append([]string{}, res.Warnings...),
	}
	for i, r := range res.Runs {
		out.Runs[i] = generatorRunOutput{ID: r.ID, Members: append([]string{}, r.Members...), Warnings: append([]string{}, r.Warnings...)}
		for _, w := range r.Warnings {
			out.Warnings = append(out.Warnings, r.ID+": "+w)
		}
	}
	return out
}

type generateListOutput struct {
	Generators []generatorOutput `json:"generators"`
}
type generatorOutput struct {
	ID        string   `json:"id"`
	DefaultOn bool     `json:"default_on"`
	Languages []string `json:"languages"`
	Manifests []string `json:"manifests"`
	Source    string   `json:"source"`
}

type newOutput struct {
	Space     spaceOutput     `json:"space"`
	Repos     []newRepoOutput `json:"repos"`
	Files     []string        `json:"files"`
	Generated generateOutput  `json:"generated"`
}
type addOutput newOutput

type newRepoOutput struct {
	Name         string   `json:"name"`
	RepoPath     string   `json:"repo_path"`
	WorktreePath string   `json:"worktree_path"`
	Symlink      bool     `json:"symlink"`
	From         string   `json:"from"`
	Secrets      []string `json:"secrets"`
	WasLink      bool     `json:"was_link"`
}

func jsonNew(res space.NewResult) newOutput {
	generated := res.Generated
	generated.Space = res.Space
	out := newOutput{
		Space:     jsonSpace(res.Space),
		Repos:     make([]newRepoOutput, len(res.Repos)),
		Files:     append([]string{}, res.Files...),
		Generated: jsonGenerate(generated, false),
	}
	for i, r := range res.Repos {
		out.Repos[i] = newRepoOutput{
			Name:         r.Entry.Name,
			RepoPath:     r.Entry.RepoPath,
			WorktreePath: r.Entry.WorktreePath,
			Symlink:      r.Entry.Symlink,
			From:         r.From,
			Secrets:      append([]string{}, r.Secrets...),
			WasLink:      r.WasLink,
		}
	}
	return out
}

type removeOutput struct {
	Space     string          `json:"space"`
	Repos     []removedOutput `json:"repos"`
	Generated generateOutput  `json:"generated"`
}
type removedOutput struct {
	Name          string `json:"name"`
	RepoPath      string `json:"repo_path"`
	WorktreePath  string `json:"worktree_path"`
	Symlink       bool   `json:"symlink"`
	Relinked      bool   `json:"relinked"`
	BranchDeleted bool   `json:"branch_deleted"`
	BranchError   string `json:"branch_error,omitempty"`
}

func jsonRemoved(list []space.Removed, deleteBranch bool) []removedOutput {
	out := make([]removedOutput, len(list))
	for i, r := range list {
		out[i] = removedOutput{
			Name:          r.Entry.Name,
			RepoPath:      r.Entry.RepoPath,
			WorktreePath:  r.Entry.WorktreePath,
			Symlink:       r.Entry.Symlink,
			Relinked:      r.Relinked,
			BranchDeleted: deleteBranch && !r.Entry.Symlink && r.BranchErr == nil,
		}
		if r.BranchErr != nil {
			out[i].BranchError = r.BranchErr.Error()
		}
	}
	return out
}

type deleteOutput struct {
	Space     spaceOutput     `json:"space"`
	Repos     []removedOutput `json:"repos"`
	Files     []string        `json:"files"`
	Generated []changeOutput  `json:"generated"`
	KeptDir   bool            `json:"kept_dir"`
}
type importWTGOutput struct {
	DryRun bool              `json:"dry_run"`
	Rows   []importRowOutput `json:"rows"`
}
type importRowOutput struct {
	Name    string `json:"name"`
	Skipped bool   `json:"skipped"`
	Reason  string `json:"reason"`
}
type pluginCheckOutput struct {
	ID         string `json:"id"`
	Protocol   int    `json:"protocol"`
	MetaPassed bool   `json:"meta_passed"`
	PlanPassed bool   `json:"plan_passed"`
}
