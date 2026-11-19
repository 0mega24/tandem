package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/0mega24/tandem/internal/space"
	"github.com/0mega24/tandem/internal/state"
)

type fakeBackend struct {
	*space.Manager
	spaces []space.SpaceStatus
}

func (f fakeBackend) Status(context.Context, []string, string) ([]space.SpaceStatus, error) {
	return f.spaces, nil
}

func press(m tea.Model, k string) tea.Model {
	var msg tea.KeyPressMsg
	switch k {
	case "enter":
		msg = tea.KeyPressMsg{Code: tea.KeyEnter}
	case "down":
		msg = tea.KeyPressMsg{Code: tea.KeyDown}
	default:
		msg = tea.KeyPressMsg{Code: rune(k[0]), Text: k}
	}
	m, _ = m.Update(msg)
	return m
}

func TestChooseSpace(t *testing.T) {
	b := fakeBackend{spaces: []space.SpaceStatus{
		{Space: state.Space{Name: "feat", Path: "/ws/feat"}},
		{Space: state.Space{Name: "fix", Path: "/ws/fix"}},
	}}
	var m tea.Model = newModel(context.Background(), b, "/")
	m, _ = m.Update(m.(model).load())

	if v := m.View().Content; !strings.Contains(v, "feat") || !strings.Contains(v, "fix") {
		t.Errorf("view = %q, want both spaces", v)
	}
	m = press(press(m, "down"), "enter")
	if got := m.(model).chosen; got != "/ws/fix" {
		t.Errorf("chosen = %q, want /ws/fix", got)
	}
}

func TestHelpAndPanels(t *testing.T) {
	m := press(newModel(context.Background(), fakeBackend{}, "/"), "?")
	for _, want := range []string{"Spaces", "Repos", "Details", "n: new", "G: dry-run"} {
		if !strings.Contains(m.View().Content, want) {
			t.Errorf("view = %q, want %q", m.View().Content, want)
		}
	}
}

func (f fakeBackend) ReadGenerated(context.Context, string, string) ([]byte, error) {
	return []byte("go 1.27\n"), nil
}
