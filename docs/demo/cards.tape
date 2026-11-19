# The README cards: one screenshot per feature. make demo.
Output docs/demo/cards.gif
Source docs/demo/theme.tape
Set Width 1200
Set Height 620

Hide
Type "source docs/demo/setup.sh" Enter
Sleep 2s
Type "tdm new my-feature api shared client >/dev/null 2>&1; clear" Enter
Sleep 2s
Show

# status: the branch, changes and commits that origin lacks, and the generated files.
Hide
Type "echo 'package api' > ~/workspaces/my-feature/api/api.go; git -C ~/workspaces/my-feature/shared commit -q --allow-empty -m wip; clear" Enter
Sleep 1s
Show
Type "tdm status -l my-feature" Enter
Sleep 1.5s
Screenshot docs/demo/card-status.png

# generate --dry-run: the plan as a diff, nothing written.
Hide
Type "printf 'module example.com/web\n\ngo 1.23\n' > ~/workspaces/my-feature/client/go.mod; clear" Enter
Sleep 1s
Show
Type "tdm generate --dry-run my-feature" Enter
Sleep 1.5s
Screenshot docs/demo/card-dryrun.png

# the TUI.
Hide
Type "clear" Enter
Show
Type "tdm ui" Enter
Sleep 4s
Type "3" Sleep 500ms Down Sleep 500ms Type "4" Sleep 1.5s
Screenshot docs/demo/card-tui.png
Type "q"
Sleep 500ms
