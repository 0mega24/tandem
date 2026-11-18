package space

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// hookScript writes a hook that appends its event, event repos and repos to log.
func hookScript(t *testing.T, dir, log string) string {
	t.Helper()
	path := filepath.Join(dir, "hook")
	script := "#!/bin/sh\nprintf '%s|%s|%s|%s|%s\\n' \"$TDM_SPACE_EVENT\" \"$TDM_SPACE_NAME\" \"$TDM_SPACE_BRANCH\" " +
		"\"$(echo \"$TDM_EVENT_REPOS\" | tr '\\n' ,)\" \"$(echo \"$TDM_REPOS\" | tr '\\n' ,)\" >> " + log + "\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestHookEvents(t *testing.T) {
	f := newFixture(t, "api", "web")
	log := filepath.Join(f.dir, "hook.log")
	f.m.cfg.Always.Run = hookScript(t, f.dir, log)
	ctx := context.Background()

	newSpaceFor(t, f, "api")
	if _, err := f.m.AddRepos(ctx, "feat", []string{"web"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.m.RemoveRepos(ctx, "feat", []string{"api"}, RemoveOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.m.DeleteSpace(ctx, "feat", DeleteOptions{}); err != nil {
		t.Fatal(err)
	}

	b, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	want := "create|feat|feat|api,|api,\n" +
		"add|feat|feat|web,|api,web,\n" +
		"remove|feat|feat|api,|web,\n" +
		"delete|feat|feat|web,|web,\n"
	if string(b) != want {
		t.Errorf("hook log =\n%s\nwant\n%s", b, want)
	}
}

func TestHookFailureIsAWarning(t *testing.T) {
	f := newFixture(t, "api")
	hook := filepath.Join(f.dir, "fail")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\necho from hook\nexit 3\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	f.m.cfg.Always.Run = hook
	var out bytes.Buffer
	f.m.SetHookOutput(&out)

	if _, err := f.m.NewSpace(context.Background(), NewOptions{Name: "feat", Repos: []string{"api"}}); err != nil {
		t.Fatalf("NewSpace() = %v, want success after a failed hook", err)
	}
	if got := out.String(); !strings.Contains(got, "from hook\n") || !strings.Contains(got, "warning: always.run") {
		t.Errorf("hook output = %q, want the hook's line and a warning", got)
	}
	if _, err := f.m.Space("feat"); err != nil {
		t.Errorf("the space is gone after a failed hook: %v", err)
	}
}
