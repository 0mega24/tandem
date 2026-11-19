# The README demo: make demo (needs bin/tdm; vhs comes from nixpkgs).
Output docs/demo/tandem.gif
Source docs/demo/theme.tape
Set Width 1200
Set Height 640

Hide
Type "source docs/demo/setup.sh" Enter
Sleep 2s
Show

Type "tdm new my-feature api shared client" Sleep 300ms Enter
Sleep 2.5s
Type "cat ~/workspaces/my-feature/go.work" Sleep 300ms Enter
Sleep 2.5s
Type "clear" Enter
Type "tdm ui" Sleep 300ms Enter
Sleep 2s
Type "3" Sleep 800ms Down Sleep 800ms Down Sleep 800ms Type "4" Sleep 1.2s Down Sleep 1.2s
Type "?" Sleep 2s Type "?" Sleep 500ms
Type "q"
Sleep 1s
