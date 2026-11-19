package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/0mega24/tandem/internal/gen"
	"github.com/0mega24/tandem/internal/space"
	"github.com/0mega24/tandem/internal/state"
)

type actionBackend struct {
	spaces  []space.SpaceStatus
	calls   []operation
	risks   []space.Risk
	err     error
	removed bool
}

func (f *actionBackend) Status(context.Context, []string, string) ([]space.SpaceStatus, error) {
	return f.spaces, f.err
}

func (f *actionBackend) NewSpace(_ context.Context, opts space.NewOptions) (space.NewResult, error) {
	f.calls = append(f.calls, operation{kind: "n", name: opts.Name, repos: opts.Repos})
	return space.NewResult{}, f.err
}

func (f *actionBackend) AddRepos(_ context.Context, name string, repos []string) (space.NewResult, error) {
	f.calls = append(f.calls, operation{kind: "a", name: name, repos: repos})
	return space.NewResult{}, f.err
}

func (f *actionBackend) RemoveRepos(_ context.Context, name string, repos []string, opts space.RemoveOptions) ([]space.Removed, space.GenerateResult, error) {
	f.calls = append(f.calls, operation{kind: "r", name: name, repos: repos})
	if len(f.risks) > 0 && !opts.Confirm(f.risks) {
		return nil, space.GenerateResult{}, space.ErrDeclined
	}
	f.removed = true
	return []space.Removed{{Entry: state.RepoEntry{Name: "api"}}}, space.GenerateResult{}, f.err
}

func (f *actionBackend) DeleteSpace(_ context.Context, name string, opts space.DeleteOptions) (space.Deleted, error) {
	f.calls = append(f.calls, operation{kind: "d", name: name})
	if len(f.risks) > 0 && !opts.Confirm(f.risks) {
		return space.Deleted{}, space.ErrDeclined
	}
	f.removed = true
	return space.Deleted{}, f.err
}

func (f *actionBackend) Generate(_ context.Context, name string, opts space.GenerateOptions) (space.GenerateResult, error) {
	kind := "g"
	if opts.DryRun {
		kind = "G"
	}
	f.calls = append(f.calls, operation{kind: kind, name: name})
	return space.GenerateResult{Changes: []gen.Change{
		{Action: gen.Write, Path: "go.work", Old: []byte("old\n"), New: []byte("new\n")},
		{Action: gen.Skip, Path: "kept", Reason: "changed by user"},
	}}, f.err
}

func (f *actionBackend) SyncRepos(_ context.Context, repos []string) ([]space.SyncResult, error) {
	f.calls = append(f.calls, operation{kind: "s", repos: repos})
	return []space.SyncResult{{Repo: space.Repo{Name: "api"}, Outcome: space.SyncSkippedDirty}}, f.err
}

func (f *actionBackend) Exec(_ context.Context, name string, argv []string, stdout, stderr io.Writer, before func(state.RepoEntry)) ([]space.ExecResult, error) {
	f.calls = append(f.calls, operation{kind: "e", name: name, repos: argv})
	before(state.RepoEntry{Name: "api"})
	_, outErr := io.WriteString(stdout, "command output\n")
	_, errErr := io.WriteString(stderr, "command stderr\n")
	return []space.ExecResult{{Entry: state.RepoEntry{Name: "api"}, Err: errors.New("exit 1")}}, errors.Join(f.err, outErr, errErr)
}

func actionModel() (model, *actionBackend) {
	f := &actionBackend{spaces: []space.SpaceStatus{{Space: state.Space{Name: "feat", Path: "/ws/feat", Branch: "feat"}, Repos: []space.MemberStatus{{Entry: state.RepoEntry{Name: "api", WorktreePath: "/ws/feat/api"}, Unpushed: 2}}}}}
	m := newModel(context.Background(), f, "/")
	loaded, _ := m.Update(m.load())
	return loaded.(model), f
}

func keyUpdate(m model, key string) (model, tea.Cmd) {
	var msg tea.KeyPressMsg
	switch key {
	case "enter":
		msg.Code = tea.KeyEnter
	case "esc":
		msg.Code = tea.KeyEscape
	case "tab":
		msg.Code = tea.KeyTab
	case "down":
		msg.Code = tea.KeyDown
	default:
		msg.Code, msg.Text = rune(key[0]), key
	}
	next, cmd := m.Update(msg)
	return next.(model), cmd
}

func typeValue(m model, value string) model {
	for _, r := range value {
		m, _ = keyUpdate(m, string(r))
	}
	return m
}

func applyResult(t *testing.T, m model, cmd tea.Cmd) model {
	t.Helper()
	if cmd == nil {
		t.Fatal("action returned no command")
	}
	if !m.busy || !strings.Contains(m.View().Content, "busy") {
		t.Fatal("operation must show busy before running")
	}
	result := cmd()
	if _, ok := result.(resultMsg); !ok {
		t.Fatalf("result = %T, want resultMsg", result)
	}
	next, refresh := m.Update(result)
	m = next.(model)
	if refresh != nil {
		next, _ = m.Update(refresh())
		m = next.(model)
	}
	return m
}

func TestActionKeys(t *testing.T) {
	for _, tc := range []struct{ key, input, kind, want string }{
		{"n", "newspace", "n", "repos added"},
		{"a", "api web", "a", "repos added"},
		{"g", "", "g", "changed by user"},
		{"G", "", "G", "+new"},
		{"s", "", "s", "skipped: local changes"},
		{"e", "printf 'hello world'", "e", "command stderr"},
	} {
		t.Run(tc.key, func(t *testing.T) {
			m, f := actionModel()
			m, cmd := keyUpdate(m, tc.key)
			if tc.input != "" {
				if m.prompt == "" {
					t.Fatal("action did not open text input")
				}
				m = typeValue(m, tc.input)
				m, cmd = keyUpdate(m, "enter")
				if tc.key == "n" {
					m = typeValue(m, "api web")
					m, cmd = keyUpdate(m, "enter")
				}
			}
			if len(f.calls) != 0 {
				t.Fatal("backend called from Update instead of Cmd")
			}
			m = applyResult(t, m, cmd)
			if len(f.calls) != 1 || f.calls[0].kind != tc.kind {
				t.Fatalf("calls = %+v", f.calls)
			}
			if !strings.Contains(m.View().Content, tc.want) {
				t.Fatalf("view = %q, want %q", m.View().Content, tc.want)
			}
			switch tc.key {
			case "n", "a":
				if !reflect.DeepEqual(f.calls[0].repos, []string{"api", "web"}) {
					t.Fatalf("repos = %v", f.calls[0].repos)
				}
				if tc.key == "n" && f.calls[0].name != "newspace" {
					t.Fatalf("name = %q", f.calls[0].name)
				}
			case "s":
				if !reflect.DeepEqual(f.calls[0].repos, []string{"api"}) {
					t.Fatalf("sync repos = %v", f.calls[0].repos)
				}
			case "e":
				if !reflect.DeepEqual(f.calls[0].repos, []string{"sh", "-c", tc.input}) {
					t.Fatalf("exec argv = %v", f.calls[0].repos)
				}
			}
		})
	}
}

func TestConfirmation(t *testing.T) {
	for _, key := range []string{"r", "d"} {
		for _, answer := range []string{"y", "n", "esc"} {
			t.Run(key+answer, func(t *testing.T) {
				m, f := actionModel()
				f.risks = []space.Risk{{Name: "api", Unpushed: 2}}
				m, _ = keyUpdate(m, key)
				if key == "r" {
					m = typeValue(m, "api")
					m, _ = keyUpdate(m, "enter")
				}
				if m.pending == nil || !strings.Contains(m.View().Content, "0 changed, 2 unpushed") {
					t.Fatalf("missing risk modal: %s", m.View().Content)
				}
				m, cmd := keyUpdate(m, answer)
				if m.pending != nil {
					t.Fatal("modal stayed open")
				}
				if answer == "y" {
					applyResult(t, m, cmd)
					if !f.removed {
						t.Fatal("confirmed operation did not remove")
					}
				} else if len(f.calls) != 0 || cmd != nil {
					t.Fatal("cancel called backend")
				}
			})
		}
	}
}

func TestChangedRiskNeedsConfirmation(t *testing.T) {
	m, f := actionModel()
	f.risks = []space.Risk{{Name: "api", Changed: 3, Unpushed: 2}}
	m, _ = keyUpdate(m, "d")
	m, cmd := keyUpdate(m, "y")
	m = applyResult(t, m, cmd)
	if f.removed || m.pending == nil || !strings.Contains(m.View().Content, "3 changed") {
		t.Fatalf("new risks not confirmed: %+v", m)
	}
	m, cmd = keyUpdate(m, "y")
	applyResult(t, m, cmd)
	if !f.removed {
		t.Fatal("second confirmation did not remove")
	}
}

func TestFilterAndCancel(t *testing.T) {
	m, f := actionModel()
	m.spaces = append(m.spaces, space.SpaceStatus{Space: state.Space{Name: "fix", Path: "/ws/fix"}})
	m, _ = keyUpdate(m, "/")
	m = typeValue(m, "FIX")
	m, _ = keyUpdate(m, "enter")
	if len(m.visible()) != 1 || m.visible()[0].Space.Name != "fix" {
		t.Fatalf("visible = %v", m.visible())
	}
	m, _ = keyUpdate(m, "enter")
	if m.chosen != "/ws/fix" {
		t.Fatalf("chosen = %q", m.chosen)
	}
	for _, key := range []string{"n", "a", "r", "e", "/"} {
		m, _ = keyUpdate(m, key)
		m = typeValue(m, "q")
		m, cmd := keyUpdate(m, "esc")
		if m.prompt != "" || cmd != nil || len(f.calls) > 0 {
			t.Fatalf("%s did not cancel", key)
		}
	}
}

func TestResultsAndBusyKeys(t *testing.T) {
	m, f := actionModel()
	f.err = errors.New("backend failed")
	m, cmd := keyUpdate(m, "g")
	busy, blocked := keyUpdate(m, "d")
	if busy.pending != nil || blocked != nil {
		t.Fatal("busy model accepted another operation")
	}
	m = applyResult(t, m, cmd)
	if !strings.Contains(m.View().Content, "backend failed") || m.busy {
		t.Fatalf("error view = %s", m.View().Content)
	}
	m, _ = keyUpdate(m, "?")
	if !m.help {
		t.Fatal("help not shown")
	}
	m, _ = keyUpdate(m, "esc")
	if m.help {
		t.Fatal("help not closed")
	}
	_, quit := keyUpdate(m, "q")
	if quit == nil {
		t.Fatal("q did not quit")
	}
}

func TestLongSpaceListKeepsSelectionVisible(t *testing.T) {
	m, _ := actionModel()
	for i := range 50 {
		m.spaces = append(m.spaces, space.SpaceStatus{Space: state.Space{Name: fmt.Sprintf("space%02d", i)}})
	}
	for range 50 {
		m, _ = keyUpdate(m, "down")
	}
	if !strings.Contains(m.View().Content, "space49") {
		t.Fatal("selected space is outside the visible panel")
	}
}

func TestDetailsCanScrollToLastLine(t *testing.T) {
	m, _ := actionModel()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	m = next.(model)
	m.details.SetContent(strings.Repeat("output\n", 40) + "last-output-line")
	m, _ = keyUpdate(m, "tab")
	m, _ = keyUpdate(m, "tab")
	for range 60 {
		m, _ = keyUpdate(m, "down")
	}
	if !strings.Contains(m.View().Content, "last-output-line") {
		t.Fatal("last output line is clipped below the Details panel")
	}
}

func (f *actionBackend) ReadGenerated(context.Context, string, string) ([]byte, error) {
	return nil, nil
}
