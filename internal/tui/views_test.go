package tui

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/0mega24/tandem/internal/gen"
	"github.com/0mega24/tandem/internal/git"
	"github.com/0mega24/tandem/internal/space"
	"github.com/0mega24/tandem/internal/state"
)

var update = flag.Bool("update", false, "update view golden")

func redesignModel() model {
	b := fakeBackend{spaces: []space.SpaceStatus{
		{Space: state.Space{Name: "feature", Path: "/ws/feature", Branch: "feat/search"}, Repos: []space.MemberStatus{
			{Entry: state.RepoEntry{Name: "api", WorktreePath: "/ws/feature/api"}, Status: git.Status{Branch: "feat/search", Changed: []string{"main.go", "search.go"}}, Unpushed: 2},
			{Entry: state.RepoEntry{Name: "web"}, Status: git.Status{Branch: "feat/search"}},
		}, Generated: []space.GeneratedStatus{
			{File: state.GeneratedFile{Path: "go.work", Generator: "go"}, State: gen.Intact},
			{File: state.GeneratedFile{Path: "package.json", Generator: "pnpm"}, State: gen.Changed},
			{File: state.GeneratedFile{Path: "Cargo.toml", Generator: "cargo"}, State: gen.Gone},
		}},
		{Space: state.Space{Name: "fix", Path: "/ws/fix", Branch: "fix/login"}},
	}}
	m := newModel(context.Background(), b, "/")
	m.version = "0.1.0" // the golden must not depend on the build's ldflags
	next, _ := m.Update(m.load())
	next, _ = next.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return next.(model)
}

func TestFocusKeys(t *testing.T) {
	m := redesignModel()
	var previous string
	for _, key := range []string{"1", "2", "3", "4"} {
		m, _ = keyUpdate(m, key)
		if got := m.panel; got != int(key[0]-'1') {
			t.Fatalf("focus %s = %d", key, got)
		}
		view := m.View().Content
		if view == previous {
			t.Fatalf("focus %s did not change view", key)
		}
		previous = view
	}
	m, _ = keyUpdate(m, "tab")
	if m.panel != 0 {
		t.Fatalf("tab focus = %d", m.panel)
	}
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if next.(model).panel != 3 {
		t.Fatalf("shift-tab focus = %d", next.(model).panel)
	}
}

func TestEmptyState(t *testing.T) {
	m := newModel(context.Background(), fakeBackend{}, "/")
	for _, want := range []string{"no space", "n: new", "tdm new <space> <repos>", "Command log"} {
		if !strings.Contains(stripANSI(m.View().Content), want) {
			t.Errorf("empty view lacks %q", want)
		}
	}
}

func TestGeneratedRows(t *testing.T) {
	m := redesignModel()
	view := stripANSI(m.View().Content)
	for _, want := range []string{"[4]─Generated", "go.work [go] ✓", "package.json [pnpm] !", "Cargo.toml [cargo] -", "api [feat/search] ! ↑2", "web [feat/search] ✓"} {
		if !strings.Contains(view, want) {
			t.Errorf("view lacks %q", want)
		}
	}
	m, _ = keyUpdate(m, "4")
	m, _ = keyUpdate(m, "down")
	if !strings.Contains(stripANSI(m.View().Content), "changed; generate skips") {
		t.Fatal("generated details do not follow selection")
	}
}

func TestSelectedRowHighlight(t *testing.T) {
	m := redesignModel()
	m, _ = keyUpdate(m, "2")
	found := false
	for line := range strings.SplitSeq(m.View().Content, "\n") {
		if strings.Contains(stripANSI(line), "│feature                             ") {
			found = true
			if !strings.Contains(line, "48;5;4") || !strings.Contains(line, "feature                             ") {
				t.Fatalf("selected row lacks full-width highlight: %q", line)
			}
		}
	}
	if !found {
		t.Fatal("selected row missing")
	}
}

func TestViewGolden(t *testing.T) {
	m := redesignModel()
	// Keep random idle tips out of the snapshot.
	m.logs = []string{"generate feature: done"}
	got := stripANSI(m.View().Content)
	lines := strings.Split(got, "\n")
	if len(lines) != 40 {
		t.Fatalf("height = %d, want 40", len(lines))
	}
	for i, line := range lines {
		if lipgloss.Width(line) != 120 {
			t.Fatalf("line %d width = %d", i, lipgloss.Width(line))
		}
	}
	path := filepath.Join("testdata", "view.golden")
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got+"\n" != string(want) {
		t.Fatalf("view differs from %s; run with -update", path)
	}
}

func TestGeneratedContent(t *testing.T) {
	m := redesignModel()
	m, cmd := keyUpdate(m, "4")
	if cmd == nil {
		t.Fatal("focusing Generated must load the selected file")
	}
	next, _ := m.Update(cmd())
	if !strings.Contains(stripANSI(next.View().Content), "go 1.27") {
		t.Fatal("generated file content missing")
	}
	m, _ = keyUpdate(m, "2")
	next, _ = m.Update(cmd())
	if strings.Contains(stripANSI(next.View().Content), "go 1.27") {
		t.Fatal("stale generated content replaced space summary")
	}
}

func stripANSI(s string) string {
	return regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAllString(s, "")
}

func TestGeneratedFilePaging(t *testing.T) {
	m := redesignModel()
	m, _ = keyUpdate(m, "4")
	next, _ := m.Update(generatedMsg{space: "feature", path: "go.work", content: []byte(strings.Repeat("use ./api\n", 40) + "last-generated-line")})
	m = next.(model)
	for range 3 {
		m, _ = keyUpdate(m, ".")
	}
	if !strings.Contains(stripANSI(m.View().Content), "last-generated-line") {
		t.Fatal("generated content did not page to the last line")
	}
	m, _ = keyUpdate(m, ",")
	if strings.Contains(stripANSI(m.View().Content), "last-generated-line") {
		t.Fatal("generated content did not page back")
	}
}
