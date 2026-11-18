package external

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/0mega24/tandem/internal/gen"
)

const (
	validMeta = `{"protocol":1,"id":"demo","languages":["Demo"],"manifests":["demo"],"default_on":false,"config_schema":{"type":"object"}}`
	emptyPlan = `{"files":[],"stale":[],"links":[],"warnings":[]}`
)

func script(t *testing.T, dir, id, body string) string {
	t.Helper()
	p := filepath.Join(dir, "tdm-gen-"+id)
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func isolate(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
}

func TestDiscover(t *testing.T) {
	isolate(t)
	first, second := t.TempDir(), t.TempDir()
	want := script(t, first, "demo", "echo '"+validMeta+"'")
	script(t, second, "demo", "exit 1")
	script(t, first, "ignored", "exit 1")
	if err := os.Chmod(filepath.Join(first, "tdm-gen-ignored"), 0o644); err != nil {
		t.Fatal(err)
	}
	plugins, err := Discover([]string{first}, second)
	if err != nil {
		t.Fatal(err)
	}
	if len(plugins) != 1 || plugins[0].Path != want || plugins[0].Meta().ID != "demo" {
		t.Fatalf("Discover = %+v, want first demo at %s", plugins, want)
	}
}

func TestMetaErrors(t *testing.T) {
	for _, tc := range []struct{ name, body, want string }{
		{"exit", "exit 1", "exit status"},
		{"json", "echo nope", "JSON"},
		{"unknown", "echo '" + strings.Replace(validMeta, `"protocol":1`, `"protocol":9`, 1) + "'", "supported versions: 1"},
		{"field", "echo '" + strings.Replace(validMeta, `"protocol":1`, `"typo":1,"protocol":1`, 1) + "'", "unknown field"},
		{"case", "echo '" + strings.Replace(validMeta, `"protocol"`, `"Protocol"`, 1) + "'", "unknown field"},
		{"null element", "echo '" + strings.Replace(validMeta, `["Demo"]`, `[null]`, 1) + "'", "must be strings"},
		{"missing", "echo '{}'", "required"},
		{"trailing", "echo '" + validMeta + " {}'", "JSON"},
		{"id", "echo '" + strings.Replace(validMeta, `"demo"`, `"other"`, 1) + "'", "ID"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			isolate(t)
			dir := t.TempDir()
			script(t, dir, "demo", tc.body)
			_, err := Discover([]string{dir}, "")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Discover error = %v, want %s", err, tc.want)
			}
		})
	}
}

func TestPlanLimits(t *testing.T) {
	for _, tc := range []struct{ name, body, want string }{
		{"timeout", "exec sleep 10", "time limit"},
		{"output", "exec head -c 16777217 /dev/zero", "stdout limit"},
		{"exit", "exit 1", "exit status"},
		{"unknown", "echo '{\"files\":[],\"stale\":[],\"links\":[],\"warnings\":[],\"typo\":1}'", "unknown field"},
		{"escape", "echo '{\"files\":[{\"path\":\"../bad\",\"content\":\"x\"}],\"stale\":[],\"links\":[],\"warnings\":[]}'", "path"},
		{"member", "echo '{\"files\":[{\"path\":\"app/file\",\"content\":\"x\"}],\"stale\":[],\"links\":[],\"warnings\":[]}'", "member"},
		{"null", "echo '{\"files\":null,\"stale\":[],\"links\":[],\"warnings\":[]}'", "required"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			isolate(t)
			dir := t.TempDir()
			script(t, dir, "demo", "if [ \"$1\" = meta ]; then echo '"+validMeta+"'; else "+tc.body+"; fi")
			plugins, err := Discover([]string{dir}, "")
			if err != nil {
				t.Fatal(err)
			}
			p := plugins[0]
			if tc.name == "timeout" {
				p.timeout = 30 * time.Millisecond
			}
			root := t.TempDir()
			_, err = p.Plan(gen.Space{Root: root, Members: []gen.Member{{Name: "app", WorktreePath: filepath.Join(root, "app")}}}, nil)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Plan error = %v, want %s", err, tc.want)
			}
		})
	}
}

func TestPlanWireAndStderr(t *testing.T) {
	isolate(t)
	dir := t.TempDir()
	script(t, dir, "demo", "if [ \"$1\" = meta ]; then echo '"+validMeta+"'; else cat >&2; printf '\\nfirst\\nlast' >&2; echo '{\"files\":[{\"path\":\"hello\",\"content\":\"hello\\n\"}],\"stale\":[],\"links\":[],\"warnings\":[]}'; fi")
	plugins, err := Discover([]string{dir}, "")
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	p := plugins[0]
	p.Stderr = &stderr
	root := t.TempDir()
	got, err := p.Plan(gen.Space{Name: "test", Root: root, Branch: "feat", Config: map[string]any{"key": true}, Members: []gen.Member{{Name: "app", WorktreePath: filepath.Join(root, "app"), OriginURL: "origin"}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Files) != 1 || string(got.Files[0].Content) != "hello\n" {
		t.Fatalf("Plan = %+v", got)
	}
	for _, want := range []string{`"space":{"name":"test"`, `"worktree":`, `"origin":"origin"`, `"config":{"key":true}`, "demo: first\ndemo: last"} {
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("stderr = %q, want %q", stderr.String(), want)
		}
	}
}

func TestExamples(t *testing.T) {
	isolate(t)
	plugins, err := Discover([]string{"../../../examples/plugins"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(plugins) != 2 {
		t.Fatalf("plugins = %d, want 2", len(plugins))
	}
	root := t.TempDir()
	member := filepath.Join(root, "app")
	if err := os.Mkdir(member, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"build.zig.zon", "app.gemspec"} {
		if err := os.WriteFile(filepath.Join(member, name), []byte(""), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range plugins {
		d, err := p.Detect(gen.Member{Name: "app", WorktreePath: member}, gen.DetectOptions{})
		if err != nil || d == nil {
			t.Fatalf("Detect = %v, %v", d, err)
		}
		got, err := p.Plan(gen.Space{Name: "test", Root: root, Members: []gen.Member{d.Member}}, []gen.Detection{*d})
		if err != nil || len(got.Files) != 1 || len(got.Files[0].Content) == 0 {
			t.Fatalf("%s Plan = %+v, %v", p.Meta().ID, got, err)
		}
		want := "--fork=../../app\n"
		if p.Meta().ID == "bundler" {
			want = "export BUNDLE_LOCAL__APP=" + member + "\n"
		}
		if !strings.Contains(string(got.Files[0].Content), want) {
			t.Fatalf("%s content = %q, want %q", p.Meta().ID, got.Files[0].Content, want)
		}
	}
}

func TestDiscoverSources(t *testing.T) {
	data, configured, pathDir := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	userDir := filepath.Join(data, "tandem", "plugins")
	if err := os.MkdirAll(userDir, 0o755); err != nil {
		t.Fatal(err)
	}
	user := script(t, userDir, "demo", "echo '"+validMeta+"'")
	script(t, pathDir, "demo", "exit 1")
	plugins, err := Discover(nil, pathDir)
	if err != nil || len(plugins) != 1 || plugins[0].Path != user {
		t.Fatalf("user discovery = %+v, %v", plugins, err)
	}
	first := script(t, configured, "demo", "echo '"+validMeta+"'")
	plugins, err = Discover([]string{filepath.Join(data, "missing"), configured}, pathDir)
	if err != nil || len(plugins) != 1 || plugins[0].Path != first {
		t.Fatalf("configured discovery = %+v, %v", plugins, err)
	}
	if err := os.Remove(user); err != nil {
		t.Fatal(err)
	}
	pathPlugin := script(t, pathDir, "demo", "echo '"+validMeta+"'")
	plugins, err = Discover(nil, pathDir)
	if err != nil || len(plugins) != 1 || plugins[0].Path != pathPlugin {
		t.Fatalf("PATH discovery = %+v, %v", plugins, err)
	}
}

func TestPlanPaths(t *testing.T) {
	root := t.TempDir()
	for _, member := range []string{"app", "lib"} {
		if err := os.Mkdir(filepath.Join(root, member), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(root, "redirect")); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, answer string
		valid        bool
	}{
		{"symlink", `{"files":[{"path":"redirect/out","content":"x"}],"stale":[],"links":[],"warnings":[]}`, false},
		{"stale", `{"files":[],"stale":["app/out"],"links":[],"warnings":[]}`, false},
		{"duplicate", `{"files":[{"path":"out","content":"a"},{"path":"out","content":"b"}],"stale":[],"links":[],"warnings":[]}`, false},
		{"bad link", fmt.Sprintf(`{"files":[],"stale":[],"links":[{"path":"app/file","target":%q}],"warnings":[]}`, filepath.Join(root, "lib")), false},
		{"absolute target", fmt.Sprintf(`{"files":[],"stale":[],"links":[{"path":"app/subprojects/lib","target":%q}],"warnings":[]}`, filepath.Join(root, "lib")), false},
		{"own member", `{"files":[],"stale":[],"links":[{"path":"app/subprojects/app","target":"../../app"}],"warnings":[]}`, false},
		{"meson link", `{"files":[],"stale":[],"links":[{"path":"app/subprojects/lib","target":"../../lib"}],"warnings":[]}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			isolate(t)
			dir := t.TempDir()
			script(t, dir, "demo", "if [ \"$1\" = meta ]; then echo '"+validMeta+"'; else echo '"+tc.answer+"'; fi")
			plugins, err := Discover([]string{dir}, "")
			if err != nil {
				t.Fatal(err)
			}
			p, err := plugins[0].Plan(gen.Space{Root: root, Members: []gen.Member{{WorktreePath: filepath.Join(root, "app")}, {WorktreePath: filepath.Join(root, "lib")}}}, nil)
			if (err == nil) != tc.valid {
				t.Fatalf("Plan error = %v, valid = %t", err, tc.valid)
			}
			if !tc.valid {
				return
			}
			// A valid plan must also pass apply, which has the same rules as for built-in plans.
			ignored := func(string) (bool, error) { return true, nil }
			if _, err := gen.Prepare(root, nil, map[string]gen.Plan{"demo": p}, []string{"app", "lib"}, false, ignored); err != nil {
				t.Fatalf("Prepare: %v", err)
			}
		})
	}
}
