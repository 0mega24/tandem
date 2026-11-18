# tandem

Tandem (`tdm`) puts one feature branch across many repos as git worktrees in one space directory,
and writes the workspace files that connect their build systems (go.work, Cargo [patch], pnpm,
Gradle, Maven, .NET, CMake, Meson, uv; plugins for more). It also has a lazygit-style TUI.
Status: early development (only `tdm version`).

## Build and test
Dev environment: the Nix flake (`nix develop`, or direnv). The make verbs enter it when needed.

| Verb | Does |
|---|---|
| `make build` | `bin/tdm` with version ldflags |
| `make test` | unit, golden and testscript tests (gotestsum); `RUN=regex` filters |
| `make lint` | golangci-lint, with depguard structure rules |
| `make fmt` / `make fmt-check` | gofumpt and nixfmt |
| `make quick` | generate-check, tidy-check, fmt-check (in series: they compare the tree) |
| `make check` | merge gate: quick, then vet, lint, test, build in parallel |
| `make check-full` | check, race, govulncheck, toolchain integration tests |
| `make audit` | govulncheck |
| `make run ARGS="..."` | run tdm |
| `make clean` | remove bin/, dist/, coverage, result |

- One package: `nix develop -c go test ./internal/<pkg>/`.
- `make nix-build` / `make nix-check`: the Nix package (its checkPhase runs the Go tests).
- After a go.mod change: `make tidy`, then set a new `vendorHash` in nix/package.nix
  (`make nix-vendor-hash` prints the steps).
- Pinned Go tools are in go.tool.mod (`go tool -modfile=go.tool.mod <tool>`). Other tools come
  from the dev shell.

## Folder map
The locked tree with one line for each directory is in docs/DESIGN.md. In short:
- `cmd/tdm/` – main only. `internal/cli/` – commands. `internal/space/` – operations for CLI and TUI.
- `internal/{config,state,git,saga,ui,tui,version}/` – one concern each.
- `internal/gen/` – generator API, registry, apply; one sub-package per generator.
- `tools/` – go generate tools (genregistry, gendocs, newgen).
- `nix/` – package, dev shells, NixOS and home-manager modules.
- `docs/spec/` – the specs. `docs/DESIGN.md` – design, prior art, changes from the spec.

## Rules for this repo
- The specs in docs/spec/ are the requirements. A change from a spec goes in the DESIGN.md table
  "Changes from the spec" in the same commit.
- The tree is locked: change docs/DESIGN.md, then the allowlist in internal/layout_test.go.
- Generators are pure: no os/exec, no cli/state/git/saga/space/tui imports (depguard). Plan does
  no I/O. Only `gen/apply.go` writes generated files.
- Never edit `zz_generated_*`, `docs/generators.md`, `schema/config.schema.json`: run `make generate`.
- No wtg code is copied. Behavior comes from the wtg docs and the specs.
- Names: command, plugin prefix and env vars use `tdm` (`TDM_*`, `tdm-gen-<id>`); all else `tandem`.
- Decision record (docs/decisions/NNNN-slug.md) for: a new dependency, a
  change to the state or config format, the plugin protocol, or the locked tree.
- Update CHANGELOG.md `## [Unreleased]` with each user-visible change.
