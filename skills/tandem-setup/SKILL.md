---
name: tandem-setup
description: Set up Tandem (tdm) for a person. Use when the person wants to install or configure tdm, write its config file, choose the repos root and the spaces root, set always.repos, always.files or always.secrets, choose generators, add a generator plugin, or add the tcd shell helper.
---

# Set up tdm

A space is a folder with a git worktree for each selected repo, all on one branch.

Ask the person for these values first. Do not guess paths:

- the directory that holds their main clones (`discovery.root_dir`),
- the directory for spaces (`spaces.root_dir`).

## 1. Install
Check `tdm version`. If it fails, show the install commands from the README and let the person choose:

```sh
nix profile install github:0mega24/tandem
```
With Home Manager or NixOS, use the `programs.tandem` module. The Home Manager module also
adds the `tcd` helper by default. The NixOS module does not add a shell helper.

## 2. Write the config

```sh
tdm config path    # the user config file
tdm config init    # write it; an existing file is not changed
tdm config         # list the files that tdm reads, lowest priority first
```
Edit the file printed by `tdm config path`. Keep the `#:schema` line: editors with Taplo use
it to check keys. An unknown key is an error. Layers, lowest priority first: built-in defaults,
`<dir>/tandem/config.toml` for each directory in `$XDG_CONFIG_DIRS`, the user file in
`$XDG_CONFIG_HOME/tandem`, then `TDM_CONFIG` or `--config`. The first directory in
`$XDG_CONFIG_DIRS` has the highest system priority. `--config` takes priority over `TDM_CONFIG`.
An explicit file adds a layer; it does not replace the system and user files. In each config
folder, `config.yaml` is read only when `config.toml` is absent. `config init` writes TOML only.

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

- A repo in `always.repos` is a symlink to its main clone unless it is selected as a worktree.
  Edits through the symlink change the main clone and are visible in every space that links it.
- A missing secrets file is skipped; the repo is still added.
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
A plugin is an executable `tdm-gen-<id>`. Put it in a directory in `generate.plugin_dirs`,
in `$XDG_DATA_HOME/tandem/plugins` (default `~/.local/share/tandem/plugins`), or on `PATH`.
Then add its ID to `generate.enabled` and run `tdm plugin check <id>`. See `docs/plugins.md`.

## 7. Shell helper (optional)
`tcd <space>` changes to a space directory. Home Manager adds it by default. To add it
yourself, put this function in the bash or zsh rc file:

```sh
tcd() { cd "$(tdm path "$1")"; }
```
`tdm completion bash` (also `zsh`, `fish`) prints a completion script. Load it with the shell
completion system.

## 8. Check

```sh
tdm config
tdm repo status
tdm generate --list
```
Tell the person what you changed and where the config file is.
