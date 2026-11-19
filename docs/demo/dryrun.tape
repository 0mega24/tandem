# README feature: tdm generate --dry-run. make demo.
Output docs/demo/dryrun.gif
Source docs/demo/theme.tape
Set Width 1200
Set Height 640

Hide
Type "source docs/demo/setup.sh" Enter
Sleep 2s
Type "tdm new my-feature api shared client >/dev/null 2>&1" Enter
Type "printf 'module example.com/client\n\ngo 1.22\n' > ~/workspaces/my-feature/client/go.mod; clear" Enter
Sleep 1s
Show

Sleep 1s
Type "tdm generate --dry-run my-feature" Sleep 500ms Enter
Sleep 9s
Hide
Type "clear" Enter
Sleep 500ms
