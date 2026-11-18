package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
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
