# 0004. The .agents directory for the Codex marketplace

Status: accepted (2026-11-18)

## Context

The repo ships agent skills as a plugin. It provides `.agents/plugins/marketplace.json`
for Codex and `.claude-plugin/marketplace.json` for Claude Code. The locked tree had no
place for the Codex marketplace file.

## Decision

- Add `.agents/` to the locked tree. It holds only `plugins/marketplace.json`, which lists the
  `tandem` plugin with the repo root as its source.
- Keep the Codex manifest in `.codex-plugin/plugin.json`.

## Consequences

- The marketplace lists the plugin as available. Installing the plugin is a separate step.
- If the manifest path changes, update `.codex-plugin/`, the locked tree and this record.
- Nothing else goes in `.agents/`.
