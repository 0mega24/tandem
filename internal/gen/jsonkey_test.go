package gen

import (
	"strings"
	"testing"
)

func keyFile(p, folders string) File {
	return File{Path: p, Key: "folders", Content: []byte(`{"folders": ` + folders + `}`)}
}

func keyOwned(p, folders string) Owned {
	return Owned{Generator: "vscode", Path: p, Key: "folders", SHA256: keyHash(mustValue(folders))}
}

func mustValue(s string) any {
	obj, _ := readObject([]byte(`{"v": ` + s + `}`))
	return obj["v"]
}

func TestKeyOwnership(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "same.code-workspace", `{"folders": [{"path": "a"}], "settings": {"x": 1}}`)
	writeFile(t, root, "update.code-workspace", `{"folders": [{"path": "a"}], "settings": {"x": 1}}`)
	writeFile(t, root, "edited.code-workspace", `{"folders": [{"path": "mine"}]}`)
	writeFile(t, root, "theirs.code-workspace", `{"settings": {}}`)
	owned := []Owned{
		keyOwned("same.code-workspace", `[{"path": "a"}]`),
		keyOwned("update.code-workspace", `[{"path": "a"}]`),
		keyOwned("edited.code-workspace", `[{"path": "a"}]`),
	}
	plans := map[string]Plan{"vscode": {Files: []File{
		keyFile("edited.code-workspace", `[{"path": "b"}]`),
		keyFile("new.code-workspace", `[{"path": "b"}]`),
		keyFile("same.code-workspace", `[{"path": "a"}]`),
		keyFile("theirs.code-workspace", `[{"path": "b"}]`),
		keyFile("update.code-workspace", `[{"path": "b"}]`),
	}}}

	a, err := Prepare(root, owned, plans, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]Action{
		"edited.code-workspace": Skip, "new.code-workspace": Write, "same.code-workspace": Unchanged,
		"theirs.code-workspace": Skip, "update.code-workspace": Write,
	}
	for _, c := range a.Changes {
		if c.Action != want[c.Path] {
			t.Errorf("%s: %v (%s), want %v", c.Path, c.Action, c.Reason, want[c.Path])
		}
	}
	run(t, a)

	if got := readFile(t, root, "update.code-workspace"); got != "{\n\t\"folders\": [\n\t\t{\n\t\t\t\"path\": \"b\"\n\t\t}\n\t],\n\t\"settings\": {\n\t\t\"x\": 1\n\t}\n}\n" {
		t.Errorf("update.code-workspace = %q, want the new folders and the old settings", got)
	}
	if got := readFile(t, root, "edited.code-workspace"); !strings.Contains(got, "mine") {
		t.Errorf("edited.code-workspace = %q, want the person's folders kept", got)
	}
	if got := readFile(t, root, "theirs.code-workspace"); got != `{"settings": {}}` {
		t.Errorf("theirs.code-workspace = %q, want it kept", got)
	}
	keys := map[string]string{}
	for _, o := range a.Owned {
		keys[o.Path] = o.Key
	}
	if keys["new.code-workspace"] != "folders" || keys["edited.code-workspace"] != "folders" {
		t.Errorf("owned = %+v, want the key recorded", a.Owned)
	}
	if _, ok := keys["theirs.code-workspace"]; ok {
		t.Error("owned a file that tdm did not write")
	}
}

func TestKeyForceKeepsACopy(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "w.code-workspace", `{"folders": [{"path": "mine"}], "settings": {}}`)
	plans := map[string]Plan{"vscode": {Files: []File{keyFile("w.code-workspace", `[{"path": "b"}]`)}}}

	a, err := Prepare(root, nil, plans, nil, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	run(t, a)
	if got := readFile(t, root, "w.code-workspace"); !strings.Contains(got, `"b"`) || !strings.Contains(got, "settings") {
		t.Errorf("w.code-workspace = %q, want the planned folders and the settings", got)
	}
	if got := readFile(t, root, "w.code-workspace"+backupSuffix); !strings.Contains(got, "mine") {
		t.Errorf("backup = %q, want the old file", got)
	}
}

func TestKeyDelete(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "only.code-workspace", `{"folders": [{"path": "a"}]}`)
	writeFile(t, root, "shared.code-workspace", `{"folders": [{"path": "a"}], "settings": {}}`)
	writeFile(t, root, "edited.code-workspace", `{"folders": [{"path": "mine"}]}`)
	owned := []Owned{
		keyOwned("edited.code-workspace", `[{"path": "a"}]`),
		keyOwned("only.code-workspace", `[{"path": "a"}]`),
		keyOwned("shared.code-workspace", `[{"path": "a"}]`),
	}

	a, err := Prepare(root, owned, map[string]Plan{"vscode": {}}, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	run(t, a)

	if got := readFile(t, root, "only.code-workspace"); got != "<missing>" {
		t.Errorf("only.code-workspace = %q, want it deleted", got)
	}
	if got := readFile(t, root, "shared.code-workspace"); got != "{\n\t\"settings\": {}\n}\n" {
		t.Errorf("shared.code-workspace = %q, want only the settings", got)
	}
	if got := readFile(t, root, "edited.code-workspace"); !strings.Contains(got, "mine") {
		t.Errorf("edited.code-workspace = %q, want it kept", got)
	}
	if len(a.Owned) != 1 || a.Owned[0].Path != "edited.code-workspace" {
		t.Errorf("owned = %+v, want only the edited file", a.Owned)
	}
}

func TestKeyPlanMustHoldOnlyTheKey(t *testing.T) {
	for _, content := range []string{`[]`, `{"folders": [], "settings": {}}`, `{"other": 1}`, `not json`} {
		f := File{Path: "w.code-workspace", Key: "folders", Content: []byte(content)}
		if _, err := Prepare(t.TempDir(), nil, map[string]Plan{"vscode": {Files: []File{f}}}, nil, false, nil); err == nil {
			t.Errorf("Prepare accepted the plan %s", content)
		}
	}
}

// VS Code writes .code-workspace files as JSONC: comments and trailing commas.
func TestKeyJSONC(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "w.code-workspace", `{
	// the folders of the space
	"folders": [{"path": "a"},],
	/* a person's settings */
	"settings": {"url": "http://x//y", "s": "a \" /* b"},
}`)
	owned := []Owned{keyOwned("w.code-workspace", `[{"path": "a"}]`)}
	plans := map[string]Plan{"vscode": {Files: []File{keyFile("w.code-workspace", `[{"path": "b"}]`)}}}

	a, err := Prepare(root, owned, plans, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if c := a.Changes[0]; c.Action != Write || !strings.Contains(c.Reason, "comments") {
		t.Fatalf("change = %v (%s), want a write that says the comments go", c.Action, c.Reason)
	}
	run(t, a)
	got := readFile(t, root, "w.code-workspace")
	for _, want := range []string{`"b"`, `"http://x//y"`, `"a \" /* b"`} {
		if !strings.Contains(got, want) {
			t.Errorf("w.code-workspace = %s, want %s", got, want)
		}
	}
}
