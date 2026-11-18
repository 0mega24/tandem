# Tandem design

Tandem (`tdm`) makes a **space**: one directory with a git worktree of each selected repo, all on
one branch. Workspace **generators** then write the files that connect the build systems of the
repos (go.work, Cargo [patch], pnpm workspace, Gradle includeBuild and others).

The full specifications are in `docs/spec/`:

- `docs/spec/project.md`: name, credit, locked structure, Go tooling, plugins, Makefile, Nix, CI.
- `docs/spec/generators.md`: the generator interface, each generator, state and ownership,
  commands, tests and phases.

The core space behavior (new, add, remove, delete, status, exec, path, repo sync and status,
config, always.*, hooks, auto-heal, saga) follows the documented behavior of wtg, with the names
of the naming table in `docs/spec/project.md`. No wtg code is copied.

## Prior art

| Tool | Idea that Tandem takes |
|---|---|
| [wtg](https://github.com/geoffamey/wtg) | One branch across many repos as git worktrees in one space directory; a generated go.work; a saga with undo steps; state files that are hints, checked against disk and git (auto-heal); always.repos, always.files, always.secrets and the always.run hook. |
| reporoot | One integration for each ecosystem, found by manifest; workspace files for npm, Go, uv and VS Code from one set of repos. |
| repoweave | One interface for each ecosystem (activate, deactivate, check, generated files); a list of generated files; an ownership check (generated header) before a delete. |

Tandem adds rules for repos that are already workspaces (nested workspaces), a plugin protocol for
other languages, and a TUI.

## Locked project structure

This tree is the only allowed layout. `internal/layout_test.go` and depguard (`.golangci.yml`)
enforce it. Change this section first, then the allowlist in the layout test, in the same PR.

```
tandem/
├── cmd/tdm/main.go          # entry point only: calls cli.Run
├── internal/
│   ├── layout_test.go        # locks this tree
│   ├── cli/                  # commands: flags, arguments, output
│   ├── config/               # load (koanf layers), defaults, validation
│   ├── state/                # space state, migration, wtg import
│   ├── git/                  # Runner interface, system git, testhelper/
│   ├── saga/                 # Do/Undo steps
│   ├── space/                # space operations: new, add, remove, delete, generate, status, exec
│   ├── tui/                  # lazygit-style terminal UI (tdm ui)
│   ├── ui/                   # terminal output
│   ├── version/              # Version, Commit, Date (set by -ldflags), VERSION
│   └── gen/
│       ├── gen.go            # Generator, Member, Space, Plan
│       ├── registry.go       # Register, All, ByID
│       ├── apply.go          # ownership check, writes, links, saga steps
│       ├── generate.go       # go:generate directives, in order
│       ├── urlnorm/          # git URL normalization
│       ├── gentest/          # golden-file test harness
│       ├── all/              # zz_generated_imports.go
│       ├── external/         # plugin protocol client
│       └── <id>/             # golang, rust, js, ts, gradle, maven, dotnet,
│           ├── <id>.go       #   cmake, meson, uv, cabal, vscode
│           ├── <id>_test.go
│           ├── doc.go        # //tandem:generator metadata
│           └── testdata/<case>/{in,want}/
├── tools/
│   ├── genregistry/          # go generate step 1: imports file
│   ├── gendocs/              # go generate step 2: docs, schema, template
│   └── newgen/               # scaffolds a new generator
├── schema/config.schema.json # generated
├── docs/                     # DESIGN.md, spec/, generators.md (generated), plugins.md, always.md,
│                             # decisions/
├── examples/plugins/         # external generators in shell and Python
├── nix/                      # package.nix, nixos-module.nix, hm-module.nix, devshells.nix
├── skills/                   # tandem/, tandem-setup/
├── .claude-plugin/  .codex-plugin/
├── .github/workflows/ci.yml
├── flake.nix  flake.lock  .envrc
├── Makefile
├── go.mod  go.sum  go.tool.mod  go.tool.sum
├── .golangci.yml  .editorconfig  .gitignore
└── LICENSE  README.md  CHANGELOG.md  AGENTS.md  CLAUDE.md -> AGENTS.md
```

Rules:

1. Only `cmd/tdm` and `tools/*` hold package main.
2. All Go code is under `internal/`. Tandem has no public Go API in version 1. The plugin protocol
   is the public extension point.
3. A generator package may import only `internal/gen`, `internal/gen/urlnorm`, the standard library
   and file-format parsers. It must not import cli, state, git, saga, space or tui, and must not
   use os/exec.
4. `internal/space` must not import `cli` or `tui`. `internal/tui` must not import `cli`. Both
   front ends call `space`, so the TUI never shells out to tdm and never copies its logic.
5. A file named `zz_generated_*.go`, `docs/generators.md` or `schema/config.schema.json` is
   generated. Do not edit it. `make quick` fails if `make generate` changes it.
6. Each generator has the same four files and the same testdata layout. `make newgen ID=<id>`
   makes them.
7. The Makefile is the only task runner.
8. AGENTS.md is the agent guide. CLAUDE.md is a symlink to it.

## Changes from the spec

| Spec | This repo | Reason |
|---|---|---|
| Go 1.26.1, `buildGoLatestModule` = Go 1.26 | `go 1.27`; nixpkgs `go_latest` is 1.27.3 | Owner: target the latest Go. |
| Tree without `space/`, `tui/` | `internal/space/`, `internal/tui/` added | The TUI and the CLI share one set of operations. |
| `subPackages = [ "cmd/tdm" ]` | `excludedPackages = [ "tools" ]` | With subPackages, checkPhase ran only the cmd/tdm tests (open item settled on the first build). |
| `generate-check`, `tidy-check` compare with HEAD | compare the tree before and after | `make check` also works on a dirty tree before a commit. |
| `make check` runs its parts in series | `quick` in series, then vet, lint, test, build with `-j4` | Project standard: the merge gate runs in parallel. |
| Config layering by hand | koanf (file, env, confmap providers) | Owner: config parity with wtg. |
| `[[generate.custom]]` | replaced by plugins | As the project spec says. |

## TUI

`tdm ui` opens a lazygit-style view: Spaces | Repos | Details panels and a log line. Keys: `n` new,
`a` add, `r` remove, `d` delete (modal confirmation with the dirty and unpushed summary), `g`
generate, `G` dry-run diff, `s` repo sync, `e` exec, `enter` prints the space path and quits (for
`tcd`), `/` filter, `?` help, `q` quit. Long operations run in the background; the view stays
responsive. Bare `tdm` prints help, so scripts do not open the TUI.
