# README feature: tdm status -l. make demo.
Output docs/demo/status.gif
Source docs/demo/theme.tape
Set Width 1200
Set Height 420

Hide
Type "source docs/demo/setup.sh" Enter
Sleep 2s
Type "tdm new my-feature api shared client >/dev/null 2>&1" Enter
Type "echo 'package api' > ~/workspaces/my-feature/api/api.go" Enter
Type "git -C ~/workspaces/my-feature/shared commit -q --allow-empty -m wip; clear" Enter
Sleep 1s
Show

Sleep 1s
Type "tdm status -l my-feature" Sleep 500ms Enter
Sleep 8s
Hide
Type "clear" Enter
Sleep 500ms
