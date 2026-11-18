# Changelog

All notable changes to this project are in this file. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project uses
[Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added
- Project skeleton: `tdm version`, shell completion, Makefile, Nix flake (package, overlay, dev
  shells, NixOS and home-manager modules), golangci-lint with depguard structure rules, layout test.
- Layered config: `/etc/xdg/tandem`, `$XDG_CONFIG_HOME/tandem`, then `TDM_CONFIG` or `--config`.
  TOML or YAML by extension. `tdm config` prints the layers; `tdm config path` prints the user file.
- `tdm repo status [-l] [repo...]` and `tdm repo sync [repo...]` for the main clones under
  `discovery.root_dir`. A repo is named by its path under the root, or by its last part when that
  is unique. Sync skips a clone with local changes or with its default branch checked out in a
  worktree, and never touches worktrees. Linked worktrees and symlinks under the root are not
  listed as repos.
- `tdm new <space> <repo>... [--branch NAME] [--path DIR]` makes one worktree on one branch in
  each repo. A missing branch starts from the local default branch; an existing one is checked out
  as it is. All checks run first, and a failed step undoes the steps before it.
- `tdm new` applies `always.repos` (symlinks to the main clones), `always.files` (copies in the
  space root) and `always.secrets` (copies from each main clone into its worktree, mode kept).
  `always.files` must be absolute paths with distinct base names.
- `tdm add <space> <repo>...` adds a worktree on the space branch in each repo, with the same
  branch rules and secrets as `new`. An `always.repos` symlink in the space becomes a worktree.
- `tdm remove [-d] <space> <repo>...` takes repos out of a space. It asks first when a worktree
  has changes or unpushed commits. An `always.repos` repo gets its symlink back; `-d` deletes the
  space branch when it is merged.
- `tdm delete [-d|-D] <space>` removes the worktrees, symlinks, `always.files` copies, the space
  directory (when nothing else is in it) and the state. It asks first about unpushed work; `-d`
  deletes merged branches and `-D` all space branches.
- `tdm path <space>` prints the space directory (for `tcd`), and completes space names.
- `tdm status [-l] [space...]` shows each repo of each space: changes and the commits that origin
  does not have. The space that holds the current directory comes first.
