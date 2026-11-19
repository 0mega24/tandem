package config

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// TestTemplateIsDefaults uncomments every setting of the template and loads it: the result must
// be the defaults, so the template cannot drift from Defaults.
func TestTemplateIsDefaults(t *testing.T) {
	tpl := Template()
	if strings.Contains(tpl, "Do not edit") {
		t.Error("the user's file must not say 'Do not edit'")
	}
	for _, table := range []string{"discovery", "spaces", "git", "always", "generate"} {
		if !strings.Contains(tpl, "\n# ["+table+"]") {
			t.Errorf("template has no [%s] table", table)
		}
	}

	setting := regexp.MustCompile(`(?m)^# (\[[a-z_.]+\]|[a-z_]+ = )`)
	body := setting.ReplaceAllString(tpl, "$1")
	// enabled: its default is "not set". Empty lists: [] decodes to an empty slice, the default is nil.
	body = regexp.MustCompile(`(?m)^(enabled = .*|[a-z_]+ = \[\].*)$`).ReplaceAllString(body, "")
	env := Env{Home: t.TempDir()}
	file := filepath.Join(env.Home, "c.toml")
	if err := os.WriteFile(file, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Load(env, file)
	if err != nil {
		t.Fatalf("load the uncommented template: %v\n%s", err, body)
	}
	want, err := Load(env, "")
	if err != nil {
		t.Fatal(err)
	}
	got.Config.Generate.Tables, want.Config.Generate.Tables = nil, nil // the generator tables are checked by gendocs
	if !reflect.DeepEqual(got.Config, want.Config) {
		t.Errorf("uncommented template =\n%+v\nwant the defaults\n%+v", got.Config, want.Config)
	}
}
