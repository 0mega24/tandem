package version

import "testing"

func TestGetUsesLdflagsFirst(t *testing.T) {
	old := [3]string{Version, Commit, Date}
	t.Cleanup(func() { Version, Commit, Date = old[0], old[1], old[2] })

	Version, Commit, Date = "1.2.3+abc", "abc", "2026-11-17T00:00:00Z"
	got := Get()
	want := Info{Version: "1.2.3+abc", Commit: "abc", Date: "2026-11-17T00:00:00Z"}
	if got != want {
		t.Errorf("Get() = %+v, want %+v", got, want)
	}
}

func TestGetFallsBackToVersionFile(t *testing.T) {
	old := Version
	t.Cleanup(func() { Version = old })

	Version = ""
	if got := Get().Version; got != "0.1.0" {
		t.Errorf("Get().Version = %q, want %q (internal/version/VERSION)", got, "0.1.0")
	}
}

func TestString(t *testing.T) {
	tests := []struct {
		in   Info
		want string
	}{
		{Info{Version: "0.1.0"}, "tdm 0.1.0"},
		{Info{Version: "0.1.0", Commit: "abc"}, "tdm 0.1.0 (abc)"},
		{Info{Version: "0.1.0", Commit: "abc", Date: "d"}, "tdm 0.1.0 (abc, d)"},
	}
	for _, tt := range tests {
		if got := tt.in.String(); got != tt.want {
			t.Errorf("%+v.String() = %q, want %q", tt.in, got, tt.want)
		}
	}
}
