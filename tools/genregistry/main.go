// Command genregistry writes internal/gen/all/zz_generated_imports.go: one blank import for each
// generator package under internal/gen/, so that each one registers itself. It reads the source
// with go/parser and imports nothing from tandem, so it still runs when the imports file names a
// generator that was deleted.
//
// go generate runs it in internal/gen (see internal/gen/generate.go). It also checks each
// generator directory: an init() that calls gen.Register, a doc.go, and the required golden cases.
package main

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// support are the packages under internal/gen/ that are not generators. internal/layout_test.go
// has the same list.
var support = map[string]bool{"all": true, "external": true, "gentest": true, "urlnorm": true}

// cases are the golden cases that each generator must have (docs/spec/generators.md, Testing).
var cases = []string{"depends", "mixed", "nested", "no-members", "one-member", "symlink"}

func main() {
	out := flag.String("out", "", "the imports file to write, relative to the current directory")
	flag.Parse()
	if *out == "" {
		fmt.Fprintln(os.Stderr, "genregistry: -out is required")
		os.Exit(2)
	}
	if err := run(".", *out); err != nil {
		fmt.Fprintln(os.Stderr, "genregistry:", err)
		os.Exit(1)
	}
}

// run writes the imports file for the generators under dir, which is internal/gen.
func run(dir, out string) error {
	pkg, err := importPath(dir)
	if err != nil {
		return err
	}
	names, err := generators(dir)
	if err != nil {
		return err
	}
	src, err := render(pkg, names)
	if err != nil {
		return err
	}
	dst := filepath.Join(dir, out)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil { //nolint:gosec // G301: a source directory in the repo
		return err
	}
	return os.WriteFile(dst, src, 0o644) //nolint:gosec // G306: a source file in the repo
}

// importPath returns the import path of dir from the module line of the go.mod above it.
func importPath(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for root := abs; ; root = filepath.Dir(root) {
		f, err := os.Open(filepath.Join(root, "go.mod")) //nolint:gosec // G304: the go.mod of this module
		if err == nil {
			mod, err := moduleLine(f)
			_ = f.Close() // read only
			if err != nil {
				return "", err
			}
			rel, err := filepath.Rel(root, abs)
			return path.Join(mod, filepath.ToSlash(rel)), err
		}
		if filepath.Dir(root) == root {
			return "", fmt.Errorf("no go.mod above %s", abs)
		}
	}
}

func moduleLine(f *os.File) (string, error) {
	s := bufio.NewScanner(f)
	for s.Scan() {
		if mod, ok := strings.CutPrefix(strings.TrimSpace(s.Text()), "module "); ok {
			return strings.Trim(strings.TrimSpace(mod), `"`), nil
		}
	}
	if err := s.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("%s has no module line", f.Name())
}

// generators returns the sorted generator directories under dir, or the problems of each.
func generators(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var names []string
	var errs []error
	for _, e := range entries {
		n := e.Name()
		if !e.IsDir() || support[n] || n == "testdata" || strings.HasPrefix(n, ".") || strings.HasPrefix(n, "_") {
			continue
		}
		problems := check(filepath.Join(dir, n))
		for _, p := range problems {
			errs = append(errs, fmt.Errorf("%s: %s", n, p))
		}
		if len(problems) == 0 {
			names = append(names, n)
		}
	}
	return names, errors.Join(errs...)
}

// check returns the problems of one generator directory.
func check(dir string) []string {
	var problems []string
	if _, err := os.Stat(filepath.Join(dir, "doc.go")); err != nil {
		problems = append(problems, "no doc.go")
	}
	for _, c := range cases {
		if fi, err := os.Stat(filepath.Join(dir, "testdata", c)); err != nil || !fi.IsDir() {
			problems = append(problems, "no golden case testdata/"+c)
		}
	}

	ok, err := registers(dir)
	switch {
	case err != nil:
		problems = append(problems, err.Error())
	case !ok:
		problems = append(problems, "no init() that calls gen.Register")
	}
	return problems
}

// registers reports whether a non-test file in dir has an init() that calls gen.Register.
func registers(dir string) (bool, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return false, err
	}

	fset := token.NewFileSet()
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, f, nil, parser.SkipObjectResolution)
		if err != nil {
			return false, err
		}
		for _, d := range file.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if ok && fn.Recv == nil && fn.Name.Name == "init" && callsRegister(fn.Body) {
				return true, nil
			}
		}
	}
	return false, nil
}

func callsRegister(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return !found
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if x, isIdent := sel.X.(*ast.Ident); ok && isIdent && x.Name == "gen" && sel.Sel.Name == "Register" {
			found = true
		}
		return !found
	})
	return found
}

func render(pkg string, names []string) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString("// Code generated by genregistry. DO NOT EDIT.\n\n")
	b.WriteString("// Package all imports every built-in generator, so that each one registers itself.\n")
	b.WriteString("package all\n")
	if len(names) > 0 {
		b.WriteString("\nimport (\n")
		for _, n := range names {
			fmt.Fprintf(&b, "\t_ %q\n", path.Join(pkg, n))
		}
		b.WriteString(")\n")
	}
	return format.Source(b.Bytes())
}
