package space

import (
	"context"
	"os"
	"os/exec"
	"strings"

	"github.com/0mega24/tandem/internal/state"
)

// The events of the always.run hook.
const (
	EventCreate = "create"
	EventAdd    = "add"
	EventRemove = "remove"
	EventDelete = "delete"
)

// runHook runs always.run after a command that succeeded. repos are the repos of the space after
// the event (before it for delete), and changed are the ones that the event added or removed.
//
// The hook is best-effort: the command has finished, so a failure is a warning, not an error,
// and nothing is undone.
func (m *Manager) runHook(ctx context.Context, sp state.Space, event string, repos, changed []state.RepoEntry) {
	if m.cfg.Always.Run == "" {
		return
	}

	cmd := exec.CommandContext(ctx, m.cfg.Always.Run) //nolint:gosec // G204: the user's own hook from the config
	if _, err := os.Stat(sp.Path); err == nil {
		cmd.Dir = sp.Path // after a delete it is gone, and the hook runs where tdm runs
	}
	cmd.Env = append(os.Environ(),
		"TDM_SPACE_NAME="+sp.Name,
		"TDM_SPACE_ROOT="+sp.Path,
		"TDM_SPACE_BRANCH="+sp.Branch,
		"TDM_SPACE_EVENT="+event,
		"TDM_REPOS="+joinEntries(repos, func(e state.RepoEntry) string { return e.Name }),
		"TDM_REPO_PATHS="+joinEntries(repos, func(e state.RepoEntry) string { return e.WorktreePath }),
		"TDM_EVENT_REPOS="+joinEntries(changed, func(e state.RepoEntry) string { return e.Name }),
		"TDM_GENERATED_FILES="+generatedPaths(sp),
	)
	cmd.Stdout, cmd.Stderr = m.out, m.out

	if err := cmd.Run(); err != nil {
		m.warn("always.run %s (%s): %v", m.cfg.Always.Run, event, err)
	}
}

// generatedPaths lists the generated files of the space, relative to its root, one per line.
func generatedPaths(sp state.Space) string {
	parts := make([]string, len(sp.Generated))
	for i, f := range sp.Generated {
		parts[i] = f.Path
	}
	return strings.Join(parts, "\n")
}

// joinEntries puts one value per line, the format of the TDM_* list variables.
func joinEntries(entries []state.RepoEntry, value func(state.RepoEntry) string) string {
	parts := make([]string, len(entries))
	for i, e := range entries {
		parts[i] = value(e)
	}
	return strings.Join(parts, "\n")
}
