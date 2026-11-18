package gen

// The go generate steps, in order (docs/spec/project.md, "What go generate makes"). gendocs
// comes in a later change.

//go:generate go run ../../tools/genregistry -out all/zz_generated_imports.go
