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
