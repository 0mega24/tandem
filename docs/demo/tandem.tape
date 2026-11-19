# The README demo: make demo (needs bin/tdm; vhs comes from nixpkgs).
Output docs/demo/tandem.gif
Source docs/demo/theme.tape
Set Width 1200
Set Height 640

Hide
Type "source docs/demo/setup.sh" Enter
Sleep 2s
Show

Sleep 1s
Type "tdm new my-feature api shared client" Sleep 500ms Enter
Sleep 5s
Type "cat ~/workspaces/my-feature/go.work" Sleep 500ms Enter
Sleep 5s
Type "clear" Enter
Type "tdm ui" Sleep 500ms Enter
Sleep 3s
Type "3" Sleep 1.5s Down Sleep 1.5s Down Sleep 1.5s
Type "4" Sleep 2.5s Down Sleep 2.5s
Type "?" Sleep 4s Type "?" Sleep 1s
Type "q"
Sleep 1s
# Clear at the end, so the loop starts again from an empty screen.
Type "clear" Enter
Sleep 1s
