// Package vscode writes a <space>.code-workspace file with a folder for each member and
// each always.repos symlink. Tandem owns only the folders key; settings and other keys stay.
// It reads JSONC, including comments and trailing commas, but writes plain JSON. Comments are
// lost when the file is rewritten. This generator is off by default.
//
//tandem:generator id=vscode manifest=any
package vscode
