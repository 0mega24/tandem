package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0mega24/tandem/internal/gen"
)

func run(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errb bytes.Buffer
	code = Run(context.Background(), append([]string{"tdm"}, args...), strings.NewReader(""), &out, &errb)
	return code, out.String(), errb.String()
}

func TestVersion(t *testing.T) {
	code, out, _ := run(t, "version")
	if code != 0 || !strings.HasPrefix(out, "tdm ") {
		t.Errorf("tdm version = %d %q, want 0 and a line that starts with \"tdm \"", code, out)
	}
}

func TestCompletion(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish"} {
		code, out, errOut := run(t, "completion", shell)
		if code != 0 || !strings.Contains(out, "tdm") {
			t.Errorf("tdm completion %s = %d, stdout %d bytes, stderr %q; want 0 and a script", shell, code, len(out), errOut)
		}
	}
}

func TestUnknownCommandFails(t *testing.T) {
	if code, _, _ := run(t, "no-such-command"); code == 0 {
		t.Errorf("tdm no-such-command = 0, want non-zero")
	}
}

func TestPluginRegistryLifetime(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("XDG_CONFIG_DIRS", home)
	t.Setenv("TDM_CONFIG", "")
	dir := filepath.Join(home, "tandem", "plugins")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	plugin := filepath.Join(dir, "tdm-gen-demo")
	if err := os.WriteFile(plugin, []byte("#!/bin/sh\necho '{\"protocol\":1,\"id\":\"demo\",\"languages\":[],\"manifests\":[],\"default_on\":false,\"config_schema\":{}}'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		code, out, errOut := run(t, "generate", "--list")
		if code != 0 || !strings.Contains(out, plugin) {
			t.Fatalf("list = %d, %q, %q", code, out, errOut)
		}
		if _, ok := gen.ByID("demo"); ok {
			t.Fatal("plugin remains registered after CLI run")
		}
	}
	if err := os.Remove(plugin); err != nil {
		t.Fatal(err)
	}
	code, out, errOut := run(t, "generate", "--list")
	if code != 0 || strings.Contains(out, "demo") {
		t.Fatalf("list after removal = %d, %q, %q", code, out, errOut)
	}
}
