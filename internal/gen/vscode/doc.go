// Package vscode writes <space>.code-workspace: one editor window with a root folder for each
// member and each always.repos symlink (generators spec, "Editor workspace").
//
// tdm owns only the "folders" key, so a person may keep "settings" and other keys in the file
// (gen.File.Key). Off by default: not every person uses the editor.
//
//tandem:generator id=vscode manifest=any
package vscode
