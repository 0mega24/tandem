// Package tui is tdm ui: a lazygit-style view of the spaces (docs/DESIGN.md, TUI). It calls the
// space operations through Backend, so it does no git or file work of its own, and tests drive
// the model with key messages and a fake Backend.
package tui

import (
	"context"
	"fmt"
	"io"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/0mega24/tandem/internal/space"
)

// Backend is the part of space.Manager that the TUI uses.
type Backend interface {
	Status(ctx context.Context, names []string, dir string) ([]space.SpaceStatus, error)
}

// Run shows the TUI until the user quits. It returns the path of the space that the user chose
// with enter, or "" (tcd uses the path: cd "$(tdm ui)").
func Run(ctx context.Context, b Backend, dir string, in io.Reader, out io.Writer) (string, error) {
	final, err := tea.NewProgram(newModel(ctx, b, dir), tea.WithContext(ctx), tea.WithInput(in), tea.WithOutput(out)).Run()
	if err != nil {
		return "", err
	}
	return final.(model).chosen, nil
}

// keys are the bindings of the TUI.
var keys = struct {
	up, down, enter, quit key.Binding
}{
	up:    key.NewBinding(key.WithKeys("up", "k")),
	down:  key.NewBinding(key.WithKeys("down", "j")),
	enter: key.NewBinding(key.WithKeys("enter")),
	quit:  key.NewBinding(key.WithKeys("q", "ctrl+c")),
}

type model struct {
	ctx     context.Context
	backend Backend
	dir     string

	spaces []space.SpaceStatus
	cursor int
	err    error
	chosen string // the path to print after quit
}

// loadedMsg carries the result of Backend.Status.
type loadedMsg struct {
	spaces []space.SpaceStatus
	err    error
}

func newModel(ctx context.Context, b Backend, dir string) model {
	return model{ctx: ctx, backend: b, dir: dir}
}

func (m model) Init() tea.Cmd { return m.load }

func (m model) load() tea.Msg {
	spaces, err := m.backend.Status(m.ctx, nil, m.dir)
	return loadedMsg{spaces, err}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case loadedMsg:
		m.spaces, m.err = msg.spaces, msg.err
		m.cursor = min(m.cursor, max(len(m.spaces)-1, 0))
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, keys.quit):
			return m, tea.Quit
		case key.Matches(msg, keys.up):
			m.cursor = max(m.cursor-1, 0)
		case key.Matches(msg, keys.down):
			m.cursor = min(m.cursor+1, max(len(m.spaces)-1, 0))
		case key.Matches(msg, keys.enter):
			if len(m.spaces) > 0 {
				m.chosen = m.spaces[m.cursor].Space.Path
				return m, tea.Quit
			}
		}
	}
	return m, nil
}

var (
	titleStyle    = lipgloss.NewStyle().Bold(true)
	selectedStyle = lipgloss.NewStyle().Reverse(true)
)

func (m model) View() tea.View {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Spaces") + "\n")
	if m.err != nil {
		fmt.Fprintf(&b, "error: %v\n", m.err)
	}
	for i, s := range m.spaces {
		line := fmt.Sprintf(" %s  %d repos ", s.Space.Name, len(s.Repos))
		if i == m.cursor {
			line = selectedStyle.Render(line)
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\nenter: open  q: quit\n")

	v := tea.NewView(b.String())
	v.AltScreen = true
	return v
}
