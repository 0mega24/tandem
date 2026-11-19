package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/0mega24/tandem/internal/gen"
	"github.com/0mega24/tandem/internal/space"
)

const helpText = "Details follow the focused panel.\n\n1: Status  2: Spaces  3: Repos  4: Generated\ntab / shift-tab: next / previous panel\nj/k or arrows: move / scroll output  ,/.: page\nn: new  a: add  r: remove  d: delete\ng: generate  G: dry-run diff  s: repo sync  e: exec (sh -c)\nenter: print path and quit  /: filter  ?: help  q: quit\nesc: cancel / close output"

func window(cursor, n, rows int) (int, int) {
	start := max(0, cursor-rows+1)
	return start, min(n, start+rows)
}

func (m model) leftWidth() int  { return max(12, m.width/3) }
func (m model) bodyHeight() int { return max(12, m.height-1) }
func (m model) listHeight() int { return (m.bodyHeight() - 3) / 3 }
func (m model) mainHeight() int { return max(3, m.bodyHeight()-7) }

func (m *model) resizeDetails() {
	m.details.SetWidth(max(1, m.width-m.leftWidth()-2))
	m.details.SetHeight(max(1, m.mainHeight()-2))
}

// frame puts titles and counts in the rounded border, so they do not consume content rows.
func frame(title, content, count string, width, height int, focused bool) string {
	inner := max(1, width-2)
	border := lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(8))
	if focused {
		border = border.Foreground(lipgloss.ANSIColor(6))
	}
	title = lipgloss.NewStyle().MaxWidth(inner).Render(title)
	top := border.Render("╭" + title + strings.Repeat("─", max(0, inner-lipgloss.Width(title))) + "╮")
	lines := strings.Split(content, "\n")
	out := []string{top}
	for i := range max(0, height-2) {
		line := ""
		if i < len(lines) {
			line = lipgloss.NewStyle().MaxWidth(inner).Render(lines[i])
		}
		out = append(out, border.Render("│")+line+strings.Repeat(" ", max(0, inner-lipgloss.Width(line)))+border.Render("│"))
	}
	count = lipgloss.NewStyle().MaxWidth(inner).Render(count)
	out = append(out, border.Render("╰"+strings.Repeat("─", max(0, inner-lipgloss.Width(count)))+count+"╯"))
	return strings.Join(out, "\n")
}

func listRows(rows []string, cursor, width, height int, focused bool) (string, string) {
	if len(rows) == 0 {
		return "No items", "0 of 0"
	}
	start, end := window(cursor, len(rows), max(1, height-2))
	visible := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		row := lipgloss.NewStyle().MaxWidth(width - 2).Render(rows[i])
		row += strings.Repeat(" ", max(0, width-2-lipgloss.Width(row)))
		if i == cursor && focused {
			row = lipgloss.NewStyle().Background(lipgloss.ANSIColor(4)).Foreground(lipgloss.ANSIColor(15)).Render(row)
		}
		visible = append(visible, row)
	}
	return strings.Join(visible, "\n"), fmt.Sprintf("%d of %d", cursor+1, len(rows))
}

func generatedMark(g space.GeneratedStatus) string {
	if g.Err != nil {
		return "✗"
	}
	switch g.State {
	case gen.Changed:
		return "!"
	case gen.Gone:
		return "-"
	default:
		return "✓"
	}
}

func (m model) sidePanels() string {
	width, height := m.leftWidth(), m.listHeight()
	status := "no space"
	var repos, generated []string
	if sp, ok := m.selected(); ok {
		status = spaceTitle(sp)
		for _, r := range sp.Repos {
			branch := r.Status.Branch
			mark := "✓"
			if r.Entry.Symlink {
				branch = "symlink"
			} else if branch == "" {
				branch = "detached"
			}
			if len(r.Status.Changed) > 0 {
				mark = "!"
			}
			if r.Err != nil {
				mark = "✗"
			}
			row := fmt.Sprintf("%s [%s] %s", r.Entry.Name, branch, mark)
			if r.Unpushed > 0 {
				row += fmt.Sprintf(" ↑%d", r.Unpushed)
			}
			repos = append(repos, row)
		}
		for _, g := range sp.Generated {
			generated = append(generated, fmt.Sprintf("%s [%s] %s", g.File.Path, g.File.Generator, generatedMark(g)))
		}
	}
	var spaces []string
	for _, sp := range m.visible() {
		spaces = append(spaces, sp.Space.Name)
	}
	title := "[2]─Spaces"
	if m.filter != "" {
		title += " / " + m.filter
	}
	spaceRows, count := listRows(spaces, m.cursor, width, height, m.panel == 1)
	repoRows, repoCount := listRows(repos, m.repoCursor, width, height, m.panel == 2)
	generatedHeight := m.bodyHeight() - 3 - 2*height
	generatedRows, generatedCount := listRows(generated, m.generatedCursor, width, generatedHeight, m.panel == 3)
	return strings.Join([]string{
		frame("[1]─Status", status, "", width, 3, m.panel == 0),
		frame(title, spaceRows, count, width, height, m.panel == 1),
		frame("[3]─Repos", repoRows, repoCount, width, height, m.panel == 2),
		frame("[4]─Generated", generatedRows, generatedCount, width, generatedHeight, m.panel == 3),
	}, "\n")
}

func (m model) mainContent() (string, string) {
	if m.pending != nil {
		return "Confirm", m.confirmationView()
	}
	if m.help {
		return "Help", helpText
	}
	if m.details.GetContent() != "" {
		return m.outputTitle, m.details.View()
	}
	sp, ok := m.selected()
	if !ok {
		if len(m.spaces) > 0 {
			return "Details", "No matching spaces. Press / to change the filter."
		}
		return "Welcome to tandem", "No spaces yet.\n\nPress n: new to make a space, or run:\n\n  tdm new <space> <repos>\n\nA space puts a feature branch across your repos.\nPress ? for help."
	}
	if m.panel == 2 && m.repoCursor < len(sp.Repos) {
		r := sp.Repos[m.repoCursor]
		content := fmt.Sprintf("%s\nbranch: %s\n%d changed, %d unpushed\n", r.Entry.WorktreePath, r.Status.Branch, len(r.Status.Changed), r.Unpushed)
		if r.Err != nil {
			content += "error: " + r.Err.Error() + "\n"
		}
		if r.Entry.Symlink {
			content += "Symlink to the main clone.\n"
		}
		if len(r.Status.Changed) == 0 {
			content += "\nWorking tree clean"
		} else {
			content += "\nChanged files\n" + strings.Join(r.Status.Changed, "\n")
		}
		return r.Entry.Name, content
	}
	if m.panel == 3 && m.generatedCursor < len(sp.Generated) {
		g := sp.Generated[m.generatedCursor]
		state := "generated"
		if g.State == gen.Changed {
			state = "changed; generate skips this file without --force"
		}
		if g.State == gen.Gone {
			state = "gone; press g to generate"
		}
		content := fmt.Sprintf("%s/%s\ngenerator: %s\n%s %s", sp.Space.Path, g.File.Path, g.File.Generator, generatedMark(g), state)
		if g.Err != nil {
			content += "\nerror: " + g.Err.Error()
		}
		if g.File.Target != "" {
			content += "\nlink target: " + g.File.Target
		}
		if m.generatedError != "" {
			content += "\nerror: " + m.generatedError
		}
		if m.generatedContent != "" {
			content += "\n\n" + m.generatedContent
		}
		return g.File.Path, content
	}
	return "Space summary", fmt.Sprintf("%s\n\n%s\n\n%d repos\n%d generated files\n\n3: inspect repos  4: inspect generated files\ng: generate  G: preview changes", spaceTitle(sp), sp.Space.Path, len(sp.Repos), len(sp.Generated))
}

func colorLines(content string) string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		color := -1
		switch {
		case strings.Contains(line, "error:"):
			color = 1
		case strings.HasPrefix(line, "+"):
			color = 2
		case strings.HasPrefix(line, "-"):
			color = 1
		}
		if color >= 0 {
			lines[i] = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(color)).Render(line)
		}
	}
	return strings.Join(lines, "\n")
}

func (m model) keyBar() string {
	keys := "n: new | a: add | d: delete | g: generate | G: diff | ?: help | q: quit"
	switch m.panel {
	case 0:
		keys = "n: new | enter: open | ?: help | q: quit"
	case 2:
		keys = "a: add | r: remove | s: sync | e: exec | ,/.: page | ?: help"
	case 3:
		keys = "g: generate | G: diff | ,/.: page | ?: help | q: quit"
	}
	if m.help {
		keys = "?: close help | esc: close | q: quit"
	}
	if m.pending != nil {
		keys = "y: confirm | n/esc: cancel"
	}
	if m.prompt != "" {
		keys = m.input.View()
	}
	v := "tdm " + m.version
	width := max(0, m.width-lipgloss.Width(v)-1)
	keys = lipgloss.NewStyle().MaxWidth(width).Render(keys)
	return keys + strings.Repeat(" ", max(1, m.width-lipgloss.Width(keys)-lipgloss.Width(v))) + v
}

func (m model) View() tea.View {
	m.resizeDetails()
	title, content := m.mainContent()
	if m.details.GetContent() == "" && m.pending == nil && !m.help {
		pager := m.details
		pager.SetContent(content)
		pager.SetYOffset(m.mainOffset)
		content = pager.View()
	}
	width := max(3, m.width-m.leftWidth())
	logTitle := "Command log"
	if m.busy {
		logTitle += " [busy]"
	}
	logs := strings.Join(m.logs, "\n")
	if logs == "" {
		logs = m.tip
	}
	right := frame(title, colorLines(content), "", width, m.mainHeight(), false) + "\n" + frame(logTitle, colorLines(logs), "", width, 7, false)
	body := lipgloss.JoinHorizontal(lipgloss.Top, m.sidePanels(), right)
	view := tea.NewView(body + "\n" + m.keyBar())
	view.AltScreen = true
	return view
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

// spaceTitle names a space, with its branch when the branch has another name.
func spaceTitle(sp space.SpaceStatus) string {
	if sp.Space.Branch == "" || sp.Space.Branch == sp.Space.Name {
		return sp.Space.Name
	}
	return sp.Space.Name + " → " + sp.Space.Branch
}
