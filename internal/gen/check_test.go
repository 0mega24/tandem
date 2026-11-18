package gen

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheck(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "file", "v1")
	writeFile(t, root, "w.code-workspace", `{"folders": [{"path": "a"}], "settings": {}}`)
	if err := os.Symlink("../lib", filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		o    Owned
		want FileState
	}{
		{"file", Owned{Path: "file", SHA256: Hash([]byte("v1"))}, Intact},
		{"file changed", Owned{Path: "file", SHA256: Hash([]byte("v0"))}, Changed},
		{"file gone", Owned{Path: "nothing", SHA256: Hash([]byte("v1"))}, Gone},
		{"key: other keys do not count", keyOwned("w.code-workspace", `[{"path": "a"}]`), Intact},
		{"key changed", keyOwned("w.code-workspace", `[{"path": "b"}]`), Changed},
		{"link", Owned{Path: "link", Target: "../lib"}, Intact},
		{"link retargeted", Owned{Path: "link", Target: "../api"}, Changed},
		{"link replaced by a file", Owned{Path: "file", Target: "../lib"}, Changed},
	} {
		got, err := Check(root, tc.o)
		if err != nil || got != tc.want {
			t.Errorf("%s: Check = %v, %v; want %v", tc.name, got, err, tc.want)
		}
	}
}
