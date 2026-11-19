package ui

import (
	"bytes"
	"testing"
)

func TestColorLine(t *testing.T) {
	cases := []struct{ in, want string }{
		{"api        main     ✓ clean", "api        main     \x1b[32m✓ clean\x1b[0m"},
		{"web  [feat]  ! 2 changed  ↑3  ↓1", "web  [feat]  \x1b[33m! 2 changed\x1b[0m  \x1b[36m↑3\x1b[0m  \x1b[36m↓1\x1b[0m"},
		{"  rust         - no members", "  rust         \x1b[2m- no members\x1b[0m"},
		{"my-feature  /ws/my-feature", "my-feature  /ws/my-feature"},
	}
	for _, c := range cases {
		if got := colorLine(c.in); got != c.want {
			t.Errorf("colorLine(%q) =\n%q\nwant\n%q", c.in, got, c.want)
		}
	}
}

func TestColorDiff(t *testing.T) {
	in := "--- go.work\n+++ go.work (planned)\n@@ -1,2 +1,3 @@\n use (\n-\t./a\n+\t./b\n"
	want := "\x1b[1m--- go.work\x1b[0m\n\x1b[1m+++ go.work (planned)\x1b[0m\n\x1b[36m@@ -1,2 +1,3 @@\x1b[0m\n" +
		" use (\n\x1b[31m-\t./a\x1b[0m\n\x1b[32m+\t./b\x1b[0m\n"
	if got := ColorDiff(in); got != want {
		t.Errorf("ColorDiff =\n%q\nwant\n%q", got, want)
	}
}

func TestNoColorForBuffer(t *testing.T) {
	if Color(&bytes.Buffer{}) {
		t.Error("Color(buffer) = true; want false, so tests and pipes get plain text")
	}
}

func TestNoColorEnv(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	if colorEnv() {
		t.Error("colorEnv() = true with NO_COLOR set")
	}
}

func TestTableColor(t *testing.T) {
	var b bytes.Buffer
	tb := newTable(&b, true)
	tb.Row("api", "✓ clean")
	tb.Row("myorg/web", "! 2 changed")
	if err := tb.Flush(); err != nil {
		t.Fatal(err)
	}
	want := "api        \x1b[32m✓ clean\x1b[0m\nmyorg/web  \x1b[33m! 2 changed\x1b[0m\n"
	if b.String() != want {
		t.Errorf("table =\n%q\nwant\n%q", b.String(), want)
	}
}

func TestPrefixes(t *testing.T) {
	if got := prefix("tdm: warning:", yellow, true); got != "\x1b[33mtdm: warning:\x1b[0m" {
		t.Errorf("colored prefix = %q", got)
	}
	if got := Warning(&bytes.Buffer{}); got != "tdm: warning:" {
		t.Errorf("Warning(buffer) = %q", got)
	}
	if got := Error(&bytes.Buffer{}); got != "tdm:" {
		t.Errorf("Error(buffer) = %q", got)
	}
}
