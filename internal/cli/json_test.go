package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestJSON(t *testing.T) {
	home := t.TempDir()
	for _, key := range []string{"HOME", "XDG_DATA_HOME", "XDG_CONFIG_HOME", "XDG_CONFIG_DIRS"} {
		t.Setenv(key, home)
	}
	t.Setenv("TDM_CONFIG", "")
	t.Setenv("WTG_CONFIG", "")
	for _, args := range [][]string{
		{"--json", "version"},
		{"version", "--json"},
		{"--json", "config", "path"},
		{"--json", "config"},
		{"--json", "status"},
		{"--json", "status", "-l"},
		{"--json", "generate", "--list"},
		{"--json", "import", "wtg"},
		{"--json", "--config", filepath.Join(home, "config.toml"), "config", "init"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			code, out, errOut := run(t, args...)
			if code != 0 || !json.Valid([]byte(out)) || !strings.HasPrefix(out, "{\n  ") || !strings.HasSuffix(out, "}\n") {
				t.Fatalf("%v = %d, %q, %q; want indented JSON object", args, code, out, errOut)
			}
			if strings.Contains(out, ": null") {
				t.Fatalf("null in output: %s", out)
			}
		})
	}
	for _, args := range [][]string{
		{"--json", "exec"},
		{"--json", "ui"},
		{"--json", "path"},
		{"--json", "no-such-command"},
		{"--json", "--unknown-flag"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			code, out, errOut := run(t, args...)
			var result struct {
				Error string `json:"error"`
			}
			if code == 0 || out != "" || !json.Valid([]byte(errOut)) || json.Unmarshal([]byte(errOut), &result) != nil || result.Error == "" {
				t.Fatalf("%v = %d, %q, %q; want JSON error on stderr", args, code, out, errOut)
			}
		})
	}
}
