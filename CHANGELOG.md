# Changelog

All notable changes to this project are in this file. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project uses
[Semantic Versioning](https://semver.org/).

## [Unreleased]

### Changed
- `tdm config init` writes every setting with its default (discovery, spaces, git, always,
  generate, plugin_dirs), and no "do not edit" line.
- `tdm new`, `add`, `remove` and `generate` print no row for a generator that found no members.
- Every warning starts with `tdm: warning:`.
- `tdm ui` shows the branch of a space only when its name differs from the space name.
- On a terminal, the marks in the output (`✓`, `!`, `-`, `↑`, `↓`), the dry-run diff and the
  warning and error prefixes have color, in the basic ANSI colors of the terminal theme.
  `NO_COLOR`, `TERM=dumb`, pipes and files get plain text.
- `tdm ui` has a lazygit-style layout: numbered Status, Spaces, Repos and Generated panels on the
  left, a main view and a command log on the right, and a key bar.

### Added
- Agent skills `tandem` and `tandem-setup`, with Claude Code and Codex plugin manifests.
- `make newgen ID=zig MANIFEST=build.zig.zon` scaffolds a generator with six golden cases and updates the registry.
- `tdm import wtg` imports spaces and shared config settings without changing worktrees; supports `--dry-run`.
- Project skeleton: `tdm version`, shell completion, Makefile, Nix flake (package, overlay, dev
  shells, NixOS and home-manager modules), golangci-lint with depguard structure rules, layout test.
- Layered config: `/etc/xdg/tandem`, `$XDG_CONFIG_HOME/tandem`, then `TDM_CONFIG` or `--config`.
  TOML or YAML by extension. `tdm config` prints the layers; `tdm config path` prints the user file.
- `tdm config init` writes the user config with a `#:schema` line and the `[generate]` settings as
  comments. `make generate` writes `docs/generators.md`, `schema/config.schema.json` and that
  template from the generators and the config types.
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
- `tdm exec <space> -- <command>` runs a command in each worktree in turn (not in symlinks). It
  goes on after a failure, lists the failures, and exits 1.
- The `always.run` hook runs after `new`, `add`, `remove` and `delete` with `TDM_SPACE_NAME`,
  `TDM_SPACE_ROOT`, `TDM_SPACE_BRANCH`, `TDM_SPACE_EVENT`, `TDM_REPOS`, `TDM_REPO_PATHS` and
  `TDM_EVENT_REPOS`. Its output goes to stderr; a failure is a warning.
- Auto-heal: `status`, `add`, `remove`, `delete` and `exec` check the worktrees of a space first.
  A deleted worktree is pruned in git and leaves the space (with a warning); a worktree that git
  does not know is repaired with `git worktree repair`.
- `tdm ui`: a terminal UI with Spaces, Repos, Details and a log. It runs new, add, remove,
  delete (with a confirmation that shows dirty and unpushed work), generate, a dry-run diff, repo
  sync and exec. enter prints the path of the chosen space, for `cd "$(tdm ui)"`.
- `tdm path <space>` prints the space directory (for `tcd`), and completes space names.
- `tdm generate [-n|-f] <space>` runs the generators of a space and writes their files; the first
  generator is `go` (`go.work`, with the highest `go` version of the members). tdm writes only files
  that it wrote and nobody changed; others are skipped, and `--force` replaces them and keeps
  `<file>.tandem-bak`. A file that no generator plans any more is deleted, with its derived files
  (`go.work.sum`). `--dry-run` writes nothing and shows a diff of each file that would change; `--list` lists the generators. The `[generate]`
  config table (`enabled`, `detect_depth`, `[generate.<id>]`) is checked against them.
- `tdm new` and `tdm add` run the generators of the space after the worktrees; a failure undoes
  the command. `tdm new --generate=<ids>|--no-generate` chooses the set, and the space keeps it.
  The `always.run` hook gets `TDM_GENERATED_FILES`.
- `tdm remove` writes the generated files again for the members that are left, and deletes the
  files of a generator with no members. `tdm delete` deletes the generated files that nobody
  changed, with their derived files; a changed file stays. In all commands, a generated file
  that cannot be deleted is a warning, not an error.
- `rust` generator: Cargo `[patch]` tables in `.cargo/config.toml` (or a root `Cargo.toml`
  workspace) for the crates of the members, also for git dependencies on another member.
- `js` generator: a root `pnpm-workspace.yaml` or a private root `package.json` with
  `workspaces` for the JavaScript and TypeScript members (pnpm, npm, yarn, bun).
- `ts` generator (off by default): a root `tsconfig.json` with `references` to the members.
- `maven` generator: an aggregator `pom.xml` with a module for each Maven member.
- `dotnet` generator: `<space>.slnx` (or `.sln`) with a folder for each .NET member.
- `cmake` generator: `.tandem/cmake/overrides.cmake` with `FETCHCONTENT_SOURCE_DIR_<NAME>` for each
  FetchContent or CPM dependency that is another member.
- `uv` generator (off by default): a virtual root `pyproject.toml` uv workspace for the Python
  members.
- `gradle` generator: a root `settings.gradle.kts` composite build with `includeBuild` for each
  Gradle member.
- `cabal` generator: a root `cabal.project` with the packages of the Haskell members.
- `meson` generator: links `subprojects/<dir>` in each consumer to the other members, only where git ignores the path.
- `vscode` generator (off by default): `<space>.code-workspace` with a folder for each member and
  always.repos symlink. tdm owns only the `folders` key; the other keys stay. It reads JSONC (comments,
  trailing commas) and writes plain JSON; the change says so when comments go.
- Generator plugins: an executable `tdm-gen-<id>` in `plugin_dirs`, the tandem plugin directory or
  `PATH` (protocol 1, docs/plugins.md). `tdm plugin check <id>` tests one. A plugin runs only when a
  command needs it, so `status`, `path` and `exec` stay fast with many plugins.
- The `go` generator also finds go.mod files below a member root, down to
  `generate.detect_depth`, with one go.work use line for each module.
- `tdm status -l` lists the generated files of each space: ✓ generated, ! changed by a person,
  or - gone.
- Releases: a `v*` tag makes a draft GitHub release with tarballs (Linux, macOS), .deb, .rpm and
  pacman packages with shell completions, and checksums. `make snapshot` builds them locally.
- A wtg state file with `go_workspace: true` becomes a generated `go.work` entry. A generated file
  that is gone leaves the state, with a warning.
- `tdm status [-l] [space...]` shows each repo of each space: changes and the commits that origin
  does not have. The space that holds the current directory comes first.
