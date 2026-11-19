// Package meson plans links under subprojects in member worktrees. A wrap-git URL must
// match another member's origin. The link lets Meson use that local worktree. Tandem creates it
// only when the destination is absent and git ignores it. Existing files and user links stay
// unchanged; Tandem can update links that it created and that nobody changed.
//
//tandem:generator id=meson manifest=meson.build
package meson
