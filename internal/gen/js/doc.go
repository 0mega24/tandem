// Package js writes pnpm-workspace.yaml or a private package.json in the space root.
// These files list the local JavaScript and TypeScript packages as workspace members. Auto mode
// reads each member's packageManager field, then its lock files, and falls back to npm.
// If members use different managers, it warns and writes no files; set generate.js.manager
// to choose one for the space.
// Run the package manager's install command from the space root.
//
//tandem:generator id=js manifest=package.json
package js
