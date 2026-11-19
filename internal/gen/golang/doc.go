// Package golang writes go.work in the space root. Each detected go.mod gets a use line,
// so Go can use the local modules together. The file uses the highest Go version found in those
// modules. generate.detect_depth controls how far below each worktree root to look for go.mod.
//
//tandem:generator id=go manifest=go.mod
package golang
