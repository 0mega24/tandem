---
name: tandem-setup
description: Set up Tandem (tdm) for a person. Use when the person wants to install or configure tdm, write its config file, choose the repos root and the spaces root, set always.repos, always.files or always.secrets, choose generators, add a generator plugin, or add the tcd shell helper.
---

# Set up tdm

Ask the person for these values first. Do not guess paths:
- the directory that holds their main clones (`discovery.root_dir`),
- the directory for spaces (`spaces.root_dir`).

## 1. Install
Check `tdm version`. If it fails, show the install commands from the README and let the person choose:
```sh
nix profile install github:0mega24/tandem
```
With Home Manager or NixOS, use the `programs.tandem` module. It also adds the `tcd` helper.

## 2. Write the config
```sh
tdm config path    # the user config file
tdm config init    # write it; an existing file is not changed
tdm config         # list the files that tdm reads, lowest priority first
```
Edit the file at `tdm config path`. Keep the `#:schema` line: editors with Taplo use it to check keys. An unknown key is an error. Layers, lowest first: built-in defaults, `$XDG_CONFIG_DIRS/tandem/config.toml`, `$XDG_CONFIG_HOME/tandem/config.toml`, then `TDM_CONFIG` or `--config`.

## 3. Set the roots
```toml
[discovery]
root_dir = "~/repos"   # main clones live here
max_depth = 2          # 1 scans direct children only

[spaces]
root_dir = "~/workspaces"

[git]
branch_prefix = ""     # for example "feat/"
```
Check: `tdm repo status`. It must list the main clones. If it lists none, raise `max_depth` or fix `root_dir`.

## 4. Set the always keys (optional)
```toml
[always]
repos = ["docs"]               # symlink to the main clone in every space
files = ["~/notes/AGENTS.md"]  # copied into the space root
secrets = [".env"]             # path in each repo; copied from the main clone into the worktree
run = ""                       # hook run in the space root after new, add, remove, delete
```
- A repo in `always.repos` has no branch. Its edits show in all spaces.
- A repo that has no file named in `secrets` is skipped.
- `run` is an executable. A failure is a warning only.

## 5. Choose generators
```toml
[generate]
enabled = ["go", "rust", "js"]   # or ["none"]
detect_depth = 2
```
- `tdm generate --list` shows the IDs, their defaults and their sources.
- Per-generator options go in `[generate.<id>]`. The list is in `docs/generators.md` of the Tandem repo.
- `--generate=go` and `--no-generate` on `tdm new` override this for one space.

## 6. Plugins (optional)
A plugin is an executable `tdm-gen-<id>`. Put it in a directory in `generate.plugin_dirs`, in `~/.local/share/tandem/plugins`, or on `PATH`. Then add its ID to `generate.enabled` and run `tdm plugin check <id>`. See `docs/plugins.md`.

## 7. Shell helper (optional)
`tcd <space>` changes to a space directory. Home Manager and NixOS add it. Without them, add this to the shell rc file:
```sh
tcd() { cd "$(tdm path "$1")"; }
```
For completion run `tdm completion bash` (also `zsh`, `fish`).

## 8. Check
```sh
tdm config
tdm repo status
tdm generate --list
```
Tell the person what you changed and where the config file is.
