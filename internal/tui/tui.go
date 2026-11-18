// Package tui provides the terminal front end to the shared space operations.
package tui

import (
	"context"
	"io"
	"slices"
	"strings"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/0mega24/tandem/internal/space"
	"github.com/0mega24/tandem/internal/state"
)

// Backend is the part of space.Manager that the TUI uses.
type Backend interface {
	Status(context.Context, []string, string) ([]space.SpaceStatus, error)
	NewSpace(context.Context, space.NewOptions) (space.NewResult, error)
	AddRepos(context.Context, string, []string) (space.NewResult, error)
	RemoveRepos(context.Context, string, []string, space.RemoveOptions) ([]space.Removed, space.GenerateResult, error)
	DeleteSpace(context.Context, string, space.DeleteOptions) (space.Deleted, error)
	Generate(context.Context, string, space.GenerateOptions) (space.GenerateResult, error)
	SyncRepos(context.Context, []string) ([]space.SyncResult, error)
	Exec(context.Context, string, []string, io.Writer, io.Writer, func(state.RepoEntry)) ([]space.ExecResult, error)
}

var _ Backend = (*space.Manager)(nil)

// Run returns the path chosen with enter; the caller prints it after the terminal is restored.
func Run(ctx context.Context, b Backend, dir string, in io.Reader, out io.Writer) (string, error) {
	final, err := tea.NewProgram(newModel(ctx, b, dir), tea.WithContext(ctx), tea.WithInput(in), tea.WithOutput(out)).Run()
	if err != nil {
		return "", err
	}
	return final.(model).chosen, nil
}

type model struct {
	ctx        context.Context
	backend    Backend
	dir        string
	spaces     []space.SpaceStatus
	cursor     int
	repoCursor int
	panel      int
	width      int
	height     int
	busy       bool
	help       bool
	chosen     string
	filter     string
	input      textinput.Model
	prompt     string
	newName    string
	pending    *operation
	risks      []space.Risk
	logs       []string
	details    viewport.Model
}

type loadedMsg struct {
	spaces []space.SpaceStatus
	err    error
}

func newModel(ctx context.Context, b Backend, dir string) model {
	return model{
		ctx: ctx, backend: b, dir: dir, busy: true, width: 100, height: 28,
		input: textinput.New(), details: viewport.New(viewport.WithWidth(45), viewport.WithHeight(9)),
	}
}

func (m model) Init() tea.Cmd { return m.load }

func (m model) load() tea.Msg {
	spaces, err := m.backend.Status(m.ctx, nil, m.dir)
	return loadedMsg{spaces, err}
}

func (m model) visible() []space.SpaceStatus {
	var out []space.SpaceStatus
	for _, sp := range m.spaces {
		if strings.Contains(strings.ToLower(sp.Space.Name), strings.ToLower(m.filter)) {
			out = append(out, sp)
		}
	}
	return out
}

func (m model) selected() (space.SpaceStatus, bool) {
	spaces := m.visible()
	if m.cursor >= len(spaces) {
		return space.SpaceStatus{}, false
	}
	return spaces[m.cursor], true
}

func (m *model) log(s string) {
	m.logs = append(m.logs, strings.Split(strings.TrimSpace(s), "\n")...)
	if len(m.logs) > 4 {
		m.logs = m.logs[len(m.logs)-4:]
	}
}

func (m *model) ask(prompt string) tea.Cmd {
	m.prompt = prompt
	m.input.SetValue("")
	m.input.Prompt = prompt + ": "
	if prompt == "filter" {
		m.input.SetValue(m.filter)
	}
	return m.input.Focus()
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case loadedMsg:
		m.busy = false
		if msg.err != nil {
			m.log("error: " + msg.err.Error())
		} else {
			m.spaces = msg.spaces
		}
		m.cursor = min(m.cursor, max(len(m.visible())-1, 0))
		m.repoCursor = 0
	case resultMsg:
		m.busy = false
		if len(msg.risks) > 0 {
			m.pending, m.risks = &msg.op, msg.risks
			m.log("Risk changed; confirm the current summary")
			return m, nil
		}
		if msg.text != "" {
			m.log(msg.text)
		}
		if msg.details != "" {
			m.details.SetContent(msg.details)
			m.details.GotoTop()
		}
		if msg.err != nil {
			m.log("error: " + msg.err.Error())
		}
		if msg.refresh {
			m.busy = true
			return m, m.load
		}
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.details.SetWidth(max(10, msg.Width/2-4))
		m.details.SetHeight(max(1, m.innerHeight()-detailsHeader))
	case tea.KeyPressMsg:
		return m.key(msg)
	}
	return m, nil
}

func (m model) key(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	if k == "ctrl+c" {
		return m, tea.Quit
	}
	if m.pending != nil {
		switch k {
		case "y":
			op := *m.pending
			op.risks = append([]space.Risk(nil), m.risks...)
			m.pending, m.risks = nil, nil
			return m.start(op)
		case "n", "esc":
			m.pending, m.risks = nil, nil
			m.log("Cancelled; nothing removed")
		}
		return m, nil
	}
	if m.prompt != "" {
		switch k {
		case "esc":
			m.prompt = ""
			m.input.Blur()
			return m, nil
		case "enter":
			return m.submit()
		default:
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}
	}
	if k == "q" {
		return m, tea.Quit
	}
	if k == "?" {
		m.help = !m.help
		return m, nil
	}
	if m.help {
		if k == "esc" {
			m.help = false
		}
		return m, nil
	}
	switch k {
	case "/":
		return m, m.ask("filter")
	case "tab":
		m.panel = (m.panel + 1) % 3
	case "up", "k", "down", "j":
		delta := 1
		if k == "up" || k == "k" {
			delta = -1
		}
		switch m.panel {
		case 0:
			m.cursor = max(0, min(m.cursor+delta, len(m.visible())-1))
			m.repoCursor = 0
		case 1:
			if sp, ok := m.selected(); ok {
				m.repoCursor = max(0, min(m.repoCursor+delta, len(sp.Repos)-1))
			}
		case 2:
			m.details, _ = m.details.Update(msg)
		}
	case "enter":
		if sp, ok := m.selected(); ok {
			m.chosen = sp.Space.Path
			return m, tea.Quit
		}
	default:
		if m.busy {
			return m, nil
		}
		if k == "n" {
			return m, m.ask("new space")
		}
		sp, ok := m.selected()
		if !ok {
			m.log("Select a space first")
			return m, nil
		}
		op := operation{kind: k, name: sp.Space.Name}
		switch k {
		case "a":
			return m, m.ask("add repos")
		case "r":
			return m, m.ask("remove repos")
		case "e":
			return m, m.ask("exec command")
		case "d":
			m.confirm(op)
		case "g", "G":
			return m.start(op)
		case "s":
			for _, r := range sp.Repos {
				op.repos = append(op.repos, r.Entry.Name)
			}
			if len(op.repos) == 0 {
				m.log("No repos to sync")
				return m, nil
			}
			return m.start(op)
		}
	}
	return m, nil
}

func (m model) submit() (tea.Model, tea.Cmd) {
	value := strings.TrimSpace(m.input.Value())
	if m.prompt == "filter" {
		m.filter, m.prompt, m.cursor, m.repoCursor = value, "", 0, 0
		m.input.Blur()
		return m, nil
	}
	if value == "" {
		m.log("Enter a value, or esc to cancel")
		return m, nil
	}
	prompt := m.prompt
	m.prompt = ""
	m.input.Blur()
	if prompt == "new space" {
		m.newName = value
		return m, m.ask("new repos")
	}
	op := operation{repos: strings.Fields(value)}
	if sp, ok := m.selected(); ok {
		op.name = sp.Space.Name
	}
	switch prompt {
	case "new repos":
		op.kind, op.name = "n", m.newName
	case "add repos":
		op.kind = "a"
	case "remove repos":
		op.kind = "r"
		m.confirm(op)
		return m, nil
	case "exec command":
		op.kind, op.command = "e", value
	}
	return m.start(op)
}

func (m *model) confirm(op operation) {
	m.pending, m.risks = &op, nil
	sp, ok := m.selected()
	if !ok {
		return
	}
	for _, r := range sp.Repos {
		if r.Entry.Symlink || (op.kind == "r" && !slices.Contains(op.repos, r.Entry.Name)) {
			continue
		}
		if r.Err != nil {
			m.log("error: " + r.Err.Error())
			m.pending = nil
			return
		}
		if len(r.Status.Changed) > 0 || r.Unpushed > 0 {
			m.risks = append(m.risks, space.Risk{Name: r.Entry.Name, Changed: len(r.Status.Changed), Unpushed: r.Unpushed})
		}
	}
}
