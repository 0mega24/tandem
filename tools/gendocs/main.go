// Command gendocs writes, from the registered generators and the config types:
//
//   - docs/generators.md: the generator table and the options of each generator,
//   - schema/config.schema.json: a JSON Schema of config.toml, for editors (Taplo reads the
//     #:schema line that tdm config init writes),
//   - internal/config/zz_generated_template.toml: the commented [generate] tables that
//     tdm config init writes.
//
// go generate runs it in internal/gen, after genregistry (see internal/gen/generate.go). The
// descriptions come from the `jsonschema:"description=..."` tag of a field, or else from its doc
// comment, so the docs and the code cannot disagree.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"github.com/0mega24/tandem/internal/config"
	"github.com/0mega24/tandem/internal/gen"
	_ "github.com/0mega24/tandem/internal/gen/all" // every built-in generator
)

func main() {
	docs := flag.String("docs", "", "the generator docs to write")
	schema := flag.String("schema", "", "the JSON Schema to write")
	template := flag.String("template", "", "the config template to write")
	flag.Parse()
	if *docs == "" || *schema == "" || *template == "" {
		fmt.Fprintln(os.Stderr, "gendocs: -docs, -schema and -template are required")
		os.Exit(2)
	}
	if err := run(*docs, *schema, *template); err != nil {
		fmt.Fprintln(os.Stderr, "gendocs:", err)
		os.Exit(1)
	}
}

// run works in internal/gen: the generator packages are its sub-directories, and the config
// package is ../config.
func run(docsFile, schemaFile, templateFile string) error {
	comments, err := fieldComments(append(packageDirs(), "../config")...)
	if err != nil {
		return err
	}
	gens := gen.All()

	schema, err := json.MarshalIndent(configSchema(gens, comments), "", "  ")
	if err != nil {
		return err
	}
	pkgDocs, err := packageDocs()
	if err != nil {
		return err
	}

	for file, b := range map[string][]byte{
		docsFile:     generatorsDoc(gens, comments, pkgDocs),
		schemaFile:   append(schema, '\n'),
		templateFile: template(gens),
	} {
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil { //nolint:gosec // G301: a directory in the repo
			return err
		}
		if err := os.WriteFile(file, b, 0o644); err != nil { //nolint:gosec // G306: a file in the repo
			return err
		}
	}
	return nil
}

// packageDirs returns the sub-directories of internal/gen that have a doc.go: the generators.
func packageDirs() []string {
	matches, _ := filepath.Glob("*/doc.go") // the pattern is valid
	dirs := make([]string, len(matches))
	for i, m := range matches {
		dirs[i] = filepath.Dir(m)
	}
	return dirs
}

// pkgDir is the directory of the package that defines v, relative to internal/gen.
func pkgDir(v any) string { return path.Base(reflect.TypeOf(v).PkgPath()) }

// fieldComments maps "<package dir>.<Type>.<Field>" to the doc comment of that struct field.
func fieldComments(dirs ...string) (map[string]string, error) {
	out := map[string]string{}
	fset := token.NewFileSet()
	for _, dir := range dirs {
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			file, err := parser.ParseFile(fset, f, nil, parser.ParseComments|parser.SkipObjectResolution)
			if err != nil {
				return nil, err
			}
			ast.Inspect(file, func(n ast.Node) bool {
				ts, ok := n.(*ast.TypeSpec)
				if !ok {
					return true
				}
				st, ok := ts.Type.(*ast.StructType)
				if !ok {
					return true
				}
				for _, fld := range st.Fields.List {
					doc := fld.Doc.Text()
					if doc == "" {
						doc = fld.Comment.Text()
					}
					for _, name := range fld.Names {
						out[path.Base(dir)+"."+ts.Name.Name+"."+name.Name] = strings.Join(strings.Fields(doc), " ")
					}
				}
				return true
			})
		}
	}
	return out, nil
}

// packageDocs maps a generator package directory to its package comment.
func packageDocs() (map[string]string, error) {
	out := map[string]string{}
	fset := token.NewFileSet()
	for _, dir := range packageDirs() {
		file, err := parser.ParseFile(fset, filepath.Join(dir, "doc.go"), nil, parser.ParseComments|parser.PackageClauseOnly)
		if err != nil {
			return nil, err
		}
		text := file.Doc.Text()
		text, _, _ = strings.Cut(text, "tandem:generator") // the directive is not prose
		out[dir] = strings.Join(strings.Fields(text), " ")
	}
	return out, nil
}

// tagInfo is what a `jsonschema` tag says: the enum values, the default and the description.
type tagInfo struct {
	enum       []string
	def, desc  string
	hasDefault bool
}

func parseTag(tag string) tagInfo {
	var t tagInfo
	for part := range strings.SplitSeq(tag, ",") {
		k, v, _ := strings.Cut(part, "=")
		switch k {
		case "enum":
			t.enum = append(t.enum, v)
		case "default":
			t.def, t.hasDefault = v, true
		case "description":
			t.desc = v
		}
	}
	return t
}

// keyName is the config key of a struct field: the name in its koanf or toml tag.
func keyName(f reflect.StructField) string {
	for _, tag := range []string{"koanf", "toml"} {
		if v, ok := f.Tag.Lookup(tag); ok {
			name, _, _ := strings.Cut(v, ",")
			return name
		}
	}
	return ""
}

// schemaOf returns the JSON Schema of a config value. def is the default value, or invalid.
func schemaOf(t reflect.Type, def reflect.Value, comments map[string]string, prefix string) map[string]any {
	s := map[string]any{}
	switch t.Kind() {
	case reflect.Struct:
		props := map[string]any{}
		for i := range t.NumField() {
			f := t.Field(i)
			name := keyName(f)
			if !f.IsExported() || name == "" || name == "-" {
				continue
			}
			var fdef reflect.Value
			if def.IsValid() {
				fdef = def.Field(i)
			}
			fs := schemaOf(f.Type, fdef, comments, prefix)
			info := parseTag(f.Tag.Get("jsonschema"))
			if len(info.enum) > 0 {
				fs["enum"] = info.enum
			}
			if _, set := fs["default"]; info.hasDefault && !set {
				fs["default"] = tagDefault(info.def)
			}
			desc := info.desc
			if desc == "" {
				desc = comments[prefix+"."+t.Name()+"."+f.Name]
			}
			if desc != "" {
				fs["description"] = desc
			}
			props[name] = fs
		}
		s["type"], s["properties"], s["additionalProperties"] = "object", props, false
		return s
	case reflect.Slice:
		s["type"], s["items"] = "array", schemaOf(t.Elem(), reflect.Value{}, comments, prefix)
	case reflect.Map:
		s["type"] = "object"
	case reflect.Bool:
		s["type"] = "boolean"
	case reflect.Int, reflect.Int64, reflect.Int32:
		s["type"] = "integer"
	default:
		s["type"] = "string"
	}
	if def.IsValid() && !def.IsZero() {
		s["default"] = def.Interface()
	}
	return s
}

func configSchema(gens []gen.Generator, comments map[string]string) map[string]any {
	root := schemaOf(reflect.TypeFor[config.Config](), reflect.ValueOf(config.Defaults()), comments, "config")
	props := root["properties"].(map[string]any)

	// [generate]: the generic keys, then one table per generator, which Tables holds at run time.
	generate := props["generate"].(map[string]any)
	gprops := generate["properties"].(map[string]any)
	ids := append(gen.IDs(), "none")
	gprops["enabled"].(map[string]any)["items"] = map[string]any{"type": "string", "enum": ids}
	gprops["detect_depth"].(map[string]any)["minimum"] = 0
	for _, g := range gens {
		m := g.Meta()
		table := map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{}}
		if m.Config != nil {
			table = schemaOf(reflect.TypeOf(m.Config), reflect.ValueOf(m.Config), comments, pkgDir(g))
		}
		table["description"] = fmt.Sprintf("Options of the %s generator (%s).", m.ID, strings.Join(m.Languages, ", "))
		gprops[m.ID] = table
	}

	root["$schema"] = "https://json-schema.org/draft/2020-12/schema"
	root["$id"] = config.SchemaURL
	root["title"] = "tandem config"
	return root
}

func generatorsDoc(gens []gen.Generator, comments, pkgDocs map[string]string) []byte {
	var b bytes.Buffer
	b.WriteString("<!-- Generated by tools/gendocs. Do not edit: run make generate. -->\n\n")
	b.WriteString("# Generators\n\n")
	b.WriteString("Each generator finds its manifests in the worktrees of a space and writes the workspace\n")
	b.WriteString("files that connect them. `tdm generate --list` shows the same table.\n\n")
	b.WriteString("| ID | Languages | Manifests | Default | Release |\n|---|---|---|---|---|\n")
	for _, g := range gens {
		m := g.Meta()
		on := "off"
		if m.DefaultOn {
			on = "on"
		}
		fmt.Fprintf(&b, "| `%s` | %s | %s | %s | %d |\n", m.ID, strings.Join(m.Languages, ", "), codeList(m.Manifests), on, m.Release)
	}

	for _, g := range gens {
		m := g.Meta()
		fmt.Fprintf(&b, "\n## %s\n\n%s\n", m.ID, pkgDocs[pkgDir(g)])
		var props map[string]any
		if m.Config != nil {
			props = schemaOf(reflect.TypeOf(m.Config), reflect.ValueOf(m.Config), comments, pkgDir(g))["properties"].(map[string]any)
		}
		if len(props) == 0 {
			fmt.Fprintf(&b, "\nNo options.\n")
			continue
		}
		fmt.Fprintf(&b, "\nOptions, in `[generate.%s]`:\n\n| Key | Type | Default | Description |\n|---|---|---|---|\n", m.ID)
		for _, k := range sortedKeys(props) {
			p := props[k].(map[string]any)
			typ := fmt.Sprint(p["type"])
			if e, ok := p["enum"].([]string); ok {
				typ = codeList(e)
			}
			fmt.Fprintf(&b, "| `%s` | %s | %s | %s |\n", k, typ, defaultText(p["default"]), p["description"])
		}
	}
	return b.Bytes()
}

func template(gens []gen.Generator) []byte {
	var b bytes.Buffer
	// No "do not edit" line: tdm config init copies this text into the user's own file.
	var defaults []string
	for _, g := range gens {
		if g.Meta().DefaultOn {
			defaults = append(defaults, fmt.Sprintf("%q", g.Meta().ID))
		}
	}
	b.WriteString("# [generate]\n")
	b.WriteString("# Generators to use. Not set: the defaults. [\"none\"] turns all generators off.\n")
	fmt.Fprintf(&b, "# enabled = [%s]\n", strings.Join(defaults, ", "))
	b.WriteString("# detect_depth = 0          # 0 = worktree root only\n")
	b.WriteString("# plugin_dirs = []          # folders searched for tdm-gen-<id> plugins before PATH\n")

	for _, g := range gens {
		m := g.Meta()
		if m.Config == nil {
			continue
		}
		fmt.Fprintf(&b, "\n# [generate.%s]\n", m.ID)
		v := reflect.ValueOf(m.Config)
		for i := range v.NumField() {
			f := v.Type().Field(i)
			if name := keyName(f); f.IsExported() && name != "" && name != "-" {
				fmt.Fprintf(&b, "# %s = %s\n", name, tomlValue(v.Field(i)))
			}
		}
	}
	return b.Bytes()
}

func tomlValue(v reflect.Value) string {
	switch v.Kind() {
	case reflect.String:
		return fmt.Sprintf("%q", v.String())
	case reflect.Slice:
		parts := make([]string, v.Len())
		for i := range v.Len() {
			parts[i] = tomlValue(v.Index(i))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	default:
		return fmt.Sprint(v.Interface())
	}
}

// tagDefault is the value of a `default=` tag: a JSON value (3, true) or else a plain string.
// The default in Meta.Config wins over the tag, as it is the value that the code uses.
func tagDefault(s string) any {
	var v any
	if json.Unmarshal([]byte(s), &v) != nil {
		return s
	}
	return v
}

func defaultText(v any) string {
	if v == nil {
		return ""
	}
	b, _ := json.Marshal(v) // config defaults are plain values
	return "`" + string(b) + "`"
}

func codeList(items []string) string {
	q := make([]string, len(items))
	for i, s := range items {
		q[i] = "`" + s + "`"
	}
	return strings.Join(q, ", ")
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
