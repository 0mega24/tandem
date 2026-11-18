# Tandem

Tandem (`tdm`) puts one feature branch across many repos. Each repo gets a git worktree in one
space directory, and Tandem writes the workspace files that let the build tools of the repos work
as one: `go.work`, Cargo `[patch]`, pnpm and npm workspaces, Gradle `includeBuild`, a Maven
aggregator, a .NET solution, CMake and Meson overrides, and uv workspaces. Plugins add more.

Status: early development. Nothing works yet except `tdm version`.

## Build

```sh
nix develop      # or direnv: .envrc has `use flake`
make build       # bin/tdm
make check       # the merge gate
```

Or with Nix only: `nix build` (result/bin/tdm), `nix run . -- version`.

## Acknowledgements

Tandem is inspired by [wtg](https://github.com/geoffamey/wtg) by Geoff Amey, which showed the idea
of one branch across many repos as git worktrees, with a generated go.work.

## License

MIT. See LICENSE.
