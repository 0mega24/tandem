// Command newgen scaffolds a generator. The make target updates the registry after it runs.
package main

import (
	"bytes"
	"embed"
	"flag"
	"fmt"
	"go/format"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
)

//go:embed templates/*
var sources embed.FS

func main() {
	id := flag.String("id", "", "the generator ID")
	manifest := flag.String("manifest", "", "the manifest filename in the worktree root")
	flag.Parse()
	if err := run(".", *id, *manifest); err != nil {
		fmt.Fprintln(os.Stderr, "newgen:", err)
		os.Exit(1)
	}
}

type generator struct {
	ID       string
	Package  string
	Manifest string
}

func run(root, id, manifest string) error {
	if !regexp.MustCompile(`^[a-z][a-z0-9-]*$`).MatchString(id) || id == "none" {
		return fmt.Errorf("invalid generator ID %q: use [a-z][a-z0-9-]*, except none", id)
	}
	if manifest == "" || manifest == "." || manifest == ".." || strings.ContainsAny(manifest, "/\\ \t\r\n") {
		return fmt.Errorf("invalid manifest %q: use a filename in the worktree root", manifest)
	}
	pkg := strings.ReplaceAll(id, "-", "_")
	if token.IsKeyword(pkg) || pkg == "main" {
		pkg += "gen"
	}
	data := generator{ID: id, Package: pkg, Manifest: manifest}
	templates, err := template.ParseFS(sources, "templates/*")
	if err != nil {
		return fmt.Errorf("parse templates: %w", err)
	}
	dir := filepath.Join(root, "internal/gen", id)
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil { //nolint:gosec // G301: source directories in the repo
		return err
	}
	// Mkdir refuses existing directories and symlinks before any files are written.
	if err := os.Mkdir(dir, 0o755); err != nil { //nolint:gosec // G301: a source directory in the repo
		return fmt.Errorf("create generator %s: %w", id, err)
	}
	for _, file := range []struct{ name, source string }{
		{"doc.go", "doc.go.tmpl"},
		{id + ".go", "generator.go.tmpl"},
		{id + "_test.go", "test.go.tmpl"},
	} {
		if err := writeTemplate(templates, data, filepath.Join(dir, file.name), file.source); err != nil {
			return err
		}
	}
	for _, name := range []string{"depends", "mixed", "nested", "no-members", "one-member", "symlink"} {
		base := filepath.Join(dir, "testdata", name)
		for _, sub := range []string{"in", "want"} {
			if err := os.MkdirAll(filepath.Join(base, sub), 0o755); err != nil { //nolint:gosec // G301: test fixtures in the repo
				return err
			}
		}
		members := []string{"app"}
		switch name {
		case "depends":
			members = append(members, "shared")
		case "symlink":
			members = append(members, "shared.symlink")
		case "nested":
			members = []string{"org--mono/nested"}
		case "no-members":
			members = nil
		}
		for _, member := range members {
			if err := writeTemplate(templates, data, filepath.Join(base, "in", member, manifest), "manifest.tmpl"); err != nil {
				return err
			}
		}
		if name == "mixed" || name == "no-members" {
			if err := writeTemplate(templates, data, filepath.Join(base, "in/plain/README.md"), "plain.tmpl"); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeTemplate(templates *template.Template, data generator, dst, name string) error {
	var b bytes.Buffer
	if err := templates.ExecuteTemplate(&b, name, data); err != nil {
		return fmt.Errorf("render %s: %w", name, err)
	}
	content := b.Bytes()
	if strings.HasSuffix(name, ".go.tmpl") {
		var err error
		content, err = format.Source(content)
		if err != nil {
			return fmt.Errorf("format %s: %w", dst, err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil { //nolint:gosec // G301: source and fixture directories in the repo
		return err
	}
	if err := os.WriteFile(dst, content, 0o644); err != nil { //nolint:gosec // G306: source and fixture files in the repo
		return fmt.Errorf("write %s: %w", dst, err)
	}
	return nil
}
