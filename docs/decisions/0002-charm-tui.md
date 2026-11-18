# 0002: Bubble Tea, Bubbles and Lip Gloss for the TUI

Status: accepted (2026-11-18)

## Context

`tdm ui` is a lazygit-style terminal UI (docs/DESIGN.md, TUI). It needs an event loop, key input,
the alternate screen, layout and styles. The approved plan lists the Charm v2 modules for this.

## Decision

Use `charm.land/bubbletea/v2` (event loop), `charm.land/bubbles/v2` (key bindings, lists,
viewports) and `charm.land/lipgloss/v2` (styles, layout), only in `internal/tui`. The TUI calls
the space operations through a small `Backend` interface, so no git or file logic moves into it.

## Consequences

- About 15 indirect modules (terminal input, ANSI, Unicode width) and a new Nix `vendorHash`.
  `golang.org/x/sys` and `golang.org/x/sync` move to the versions that Bubble Tea needs.
- The view draws on stderr; stdout has only the chosen path, for `cd "$(tdm ui)"`.
- Model tests send key messages to `Update` with a fake Backend; no terminal is needed.
