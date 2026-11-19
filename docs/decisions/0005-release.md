# 0005. Release with goreleaser and nfpm packages

Status: accepted (2026-11-18)

## Context

The spec names goreleaser for the release job on a `v*` tag and keeps it for "a later step".
People also want distro packages, not only archives.

## Decision

- `.goreleaser.yaml` at the root (added to the locked tree): Linux and macOS, amd64 and arm64,
  tarballs, checksums, and nfpm packages: .deb, .rpm and archlinux (pacman). The packages hold the
  bash, zsh and fish completions and the license, and depend on git.
- `.github/workflows/release.yml` runs `make check`, then `make release`. The release is a draft:
  a person reads the notes and the files, then publishes it.
- Do not build an AppImage. Linux binaries use `CGO_ENABLED=0`. Tarballs provide a direct
  install for the supported Linux architectures without a separate bundling tool.

## Consequences

- `make snapshot` builds every file locally in dist/, so a release can be checked before a tag.
- Nix users do not need these files: the flake is the Nix install.
