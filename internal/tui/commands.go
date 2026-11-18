package tui

import (
	"bytes"
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/0mega24/tandem/internal/gen"
	"github.com/0mega24/tandem/internal/space"
	"github.com/0mega24/tandem/internal/state"
	"github.com/0mega24/tandem/internal/ui"
)

type operation struct {
	kind    string
	name    string
	repos   []string
	command string
	risks   []space.Risk
}

type resultMsg struct {
	op      operation
	risks   []space.Risk
	text    string
	details string
	err     error
	refresh bool
}

func (m model) start(op operation) (tea.Model, tea.Cmd) {
	m.busy = true
	return m, func() tea.Msg { return m.perform(op) }
}

func (m model) perform(op operation) resultMsg {
	res := resultMsg{op: op, refresh: op.kind != "G"}
	var out strings.Builder
	// Manager checks risks again just before removal. Never approve work absent from the modal.
	confirm := func(risks []space.Risk) bool {
		if slices.Equal(risks, op.risks) {
			return true
		}
		res.risks = append([]space.Risk(nil), risks...)
		return false
	}
	switch op.kind {
	case "n", "a":
		var made space.NewResult
		if op.kind == "n" {
			made, res.err = m.backend.NewSpace(m.ctx, space.NewOptions{Name: op.name, Repos: op.repos})
		} else {
			made, res.err = m.backend.AddRepos(m.ctx, op.name, op.repos)
		}
		fmt.Fprintf(&out, "%s: %d repos added\n", op.name, len(made.Repos))
		for _, r := range made.Repos {
			fmt.Fprintf(&out, "%s: %s\n", r.Entry.Name, r.Entry.WorktreePath)
		}
		out.WriteString(generatedText(made.Generated, false))
	case "r":
		var removed []space.Removed
		var generated space.GenerateResult
		removed, generated, res.err = m.backend.RemoveRepos(m.ctx, op.name, op.repos, space.RemoveOptions{Confirm: confirm})
		fmt.Fprintf(&out, "%s: %d repos removed\n", op.name, len(removed))
		out.WriteString(generatedText(generated, false))
	case "d":
		var deleted space.Deleted
		deleted, res.err = m.backend.DeleteSpace(m.ctx, op.name, space.DeleteOptions{Confirm: confirm})
		fmt.Fprintf(&out, "%s: %d repos removed\n", op.name, len(deleted.Repos))
		if deleted.KeptDir {
			out.WriteString("Space directory kept: other files remain\n")
		}
	case "g", "G":
		var generated space.GenerateResult
		generated, res.err = m.backend.Generate(m.ctx, op.name, space.GenerateOptions{DryRun: op.kind == "G"})
		out.WriteString(generatedText(generated, op.kind == "G"))
	case "s":
		var synced []space.SyncResult
		synced, res.err = m.backend.SyncRepos(m.ctx, op.repos)
		for _, r := range synced {
			fmt.Fprintf(&out, "%s: %s", r.Repo.Name, r.Outcome)
			if r.Err != nil {
				fmt.Fprintf(&out, ": %v", r.Err)
			}
			out.WriteByte('\n')
		}
	case "e":
		var output bytes.Buffer
		var ran []space.ExecResult
		ran, res.err = m.backend.Exec(m.ctx, op.name, []string{"sh", "-c", op.command}, &output, &output, func(e state.RepoEntry) {
			fmt.Fprintf(&output, "[%s]\n", e.Name)
		})
		out.Write(output.Bytes())
		for _, r := range ran {
			if r.Err != nil {
				fmt.Fprintf(&out, "%s: error: %v\n", r.Entry.Name, r.Err)
			}
		}
	}
	res.text = strings.TrimSpace(out.String())
	if res.text == "" && res.err == nil {
		res.text = "Done"
	}
	res.details = res.text
	return res
}

func generatedText(res space.GenerateResult, dry bool) string {
	var out strings.Builder
	for _, r := range res.Runs {
		fmt.Fprintf(&out, "%s: %s\n", r.ID, strings.Join(r.Members, ", "))
		for _, w := range r.Warnings {
			fmt.Fprintf(&out, "warning: %s\n", w)
		}
	}
	for _, c := range res.Changes {
		fmt.Fprintf(&out, "%s %s: %s\n", c.Action, c.Path, c.Reason)
		if dry {
			switch c.Action {
			case gen.Write:
				out.WriteString(ui.Diff(c.Path, c.Old, c.New))
			case gen.Delete:
				out.WriteString(ui.Diff(c.Path, c.Old, nil))
			}
		}
	}
	for _, w := range res.Warnings {
		fmt.Fprintf(&out, "warning: %s\n", w)
	}
	if dry {
		out.WriteString("dry run: nothing was written\n")
	}
	return out.String()
}
