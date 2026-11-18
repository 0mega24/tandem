package urlnorm

import "testing"

func TestNormalize(t *testing.T) {
	want := "github.com/MyOrg/api"
	for _, u := range []string{
		"git@github.com:MyOrg/api.git",
		"git@github.com:MyOrg/api",
		"https://github.com/MyOrg/api.git",
		"https://github.com/MyOrg/api/",
		"https://user@GitHub.com/MyOrg/api",
		"ssh://git@github.com/MyOrg/api.git",
		"git://github.com/MyOrg/api",
		"  https://github.com/MyOrg/api.git/  ",
	} {
		if got := Normalize(u); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", u, got, want)
		}
	}
}

func TestSame(t *testing.T) {
	if !Same("git@gitlab.com:a/b.git", "https://gitlab.com/a/b") {
		t.Error("scp and https forms differ")
	}
	if Same("https://github.com/a/b", "https://github.com/a/c") {
		t.Error("different repos are the same")
	}
	if Same("", "") {
		t.Error("two empty URLs are the same")
	}
}
