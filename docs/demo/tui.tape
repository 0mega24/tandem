# README feature: a tour of tdm ui. make demo.
Output docs/demo/tui.gif
Source docs/demo/theme.tape
Set Width 1200
Set Height 640

Hide
Type "source docs/demo/setup.sh" Enter
Sleep 2s
Type "tdm new my-feature api shared client >/dev/null 2>&1" Enter
Type "tdm new bugfix api >/dev/null 2>&1" Enter
Type "echo 'package api' > ~/workspaces/my-feature/api/api.go" Enter
Type "printf 'module example.com/client\n\ngo 1.22\n' > ~/workspaces/my-feature/client/go.mod; clear" Enter
Sleep 1s
Show

Sleep 1s
Type "tdm ui" Sleep 500ms Enter
Sleep 3s
Type "2" Sleep 1.5s Down Sleep 2.5s
Type "3" Sleep 1.5s Down Sleep 2s Down Sleep 2s
Type "4" Sleep 2s Down Sleep 3s
Type "G" Sleep 4s
Type "?" Sleep 4s Type "?" Sleep 1s
Type "q"
Hide
Type "clear" Enter
Sleep 500ms
