package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// detailsHeader is the most lines that the Details panel shows above its viewport: the title, the
// space path and branch, and the repo name, path, branch, counts and error.
const detailsHeader = 8

const helpText = "n: new  a: add  r: remove  d: delete\ng: generate  G: dry-run diff  s: repo sync  e: exec (sh -c)\nenter: print path and quit  /: filter  ?: help  q: quit\ntab: next panel  j/k or arrows: move / scroll Details  esc: cancel"

// panelHeight is the height of a panel with its border: lipgloss v2 counts the border in Height.
func (m model) panelHeight() int { return max(5, m.height-11) }

// innerHeight is the number of text lines inside a panel.
func (m model) innerHeight() int { return m.panelHeight() - 2 }

// window returns the rows [start, end) of a list of n rows that show the cursor in a panel of rows.
func window(cursor, n, rows int) (int, int) {
	start := max(0, cursor-rows+1)
	return start, min(n, start+rows)
}

func (m model) View() tea.View {
	panelHeight := m.panelHeight()
	var spaces, repos, details strings.Builder
	spaces.WriteString("Spaces\n")
	if m.filter != "" {
		fmt.Fprintf(&spaces, "filter: %s\n", m.filter)
	}
	visible := m.visible()
	spaceRows := m.innerHeight() - 1 // the title
	if m.filter != "" {
		spaceRows--
	}
	start, end := window(m.cursor, len(visible), spaceRows)
	for i := start; i < end; i++ {
		sp := visible[i]
		mark := "  "
		if i == m.cursor {
			mark = "> "
		}
		fmt.Fprintf(&spaces, "%s%s\n", mark, sp.Space.Name)
	}
	if len(visible) == 0 {
		spaces.WriteString("No spaces\n")
	}
	repos.WriteString("Repos\n")
	details.WriteString("Details\n")
	if sp, ok := m.selected(); ok {
		fmt.Fprintf(&details, "%s\n%s\n", sp.Space.Path, sp.Space.Branch)
		first, last := window(m.repoCursor, len(sp.Repos), m.innerHeight()-1)
		for i, r := range sp.Repos {
			mark := "  "
			if i == m.repoCursor {
				mark = "> "
			}
			kind := "worktree"
			if r.Entry.Symlink {
				kind = "symlink"
			}
			if i >= first && i < last {
				fmt.Fprintf(&repos, "%s%s (%s)\n", mark, r.Entry.Name, kind)
			}
			if i == m.repoCursor {
				fmt.Fprintf(&details, "%s\n%s\nbranch: %s\n%d changed, %d unpushed\n", r.Entry.Name, r.Entry.WorktreePath, r.Status.Branch, len(r.Status.Changed), r.Unpushed)
				if r.Err != nil {
					fmt.Fprintf(&details, "error: %v\n", r.Err)
				}
			}
		}
	}
	details.WriteString(m.details.View())
	widths := []int{max(5, m.width/4-2), max(5, m.width/4-2), max(10, m.width/2-2)}
	columns := []string{spaces.String(), repos.String(), details.String()}
	for i := range columns {
		style := lipgloss.NewStyle().Width(widths[i]).Height(panelHeight).MaxHeight(panelHeight).Border(lipgloss.NormalBorder())
		if m.panel == i {
			style = style.BorderForeground(lipgloss.Color("6"))
		}
		columns[i] = style.Render(columns[i])
	}
	var out strings.Builder
	body := lipgloss.JoinHorizontal(lipgloss.Top, columns...)
	if m.help {
		body = lipgloss.Place(m.width, panelHeight, lipgloss.Center, lipgloss.Center, "Spaces | Repos | Details\n\n"+helpText)
	}
	if m.pending != nil {
		body = lipgloss.Place(m.width, panelHeight, lipgloss.Center, lipgloss.Center, m.confirmationView())
	}
	out.WriteString(body)
	out.WriteString("\nLog")
	if m.busy {
		out.WriteString(" [busy]")
	}
	out.WriteByte('\n')
	out.WriteString(strings.Join(m.logs, "\n"))
	out.WriteString("\n?: help  tab: panel  enter: open  q: quit\n")
	if m.prompt != "" {
		out.WriteString(m.input.View() + "\n")
	}

	v := tea.NewView(out.String())
	v.AltScreen = true
	return v
}

func actionName(kind string) string {
	if kind == "d" {
		return "delete"
	}
	return "remove"
}

func (m model) confirmationView() string {
	var out strings.Builder
	fmt.Fprintf(&out, "Confirm %s %s %s?\n", actionName(m.pending.kind), m.pending.name, strings.Join(m.pending.repos, " "))
	if len(m.risks) == 0 {
		out.WriteString("No known dirty or unpushed work\n")
	}
	for _, r := range m.risks {
		fmt.Fprintf(&out, "%s: %d changed, %d unpushed\n", r.Name, r.Changed, r.Unpushed)
	}
	out.WriteString("y: yes  n/esc: no\n")
	return out.String()
}
