# Records the demo. Run from the repo root with tdm on PATH: vhs docs/demo/tandem.tape
Output docs/demo/tandem.gif

Set Shell "bash"
Set FontSize 16
Set Width 1100
Set Height 640
Set Padding 16
Set TypingSpeed 60ms

# Scratch HOME with three small repos, so the demo never touches real config or clones.
Hide
Type `export HOME=$(mktemp -d) XDG_CONFIG_DIRS=/nonexistent GIT_AUTHOR_NAME=d GIT_AUTHOR_EMAIL=d@x GIT_COMMITTER_NAME=d GIT_COMMITTER_EMAIL=d@x`
Enter
Type `mkdir ~/repos && cd ~/repos && for r in api shared; do git init -q -b main $r; printf "module example.com/$r\n\ngo 1.22\n" > $r/go.mod; git -C $r add -A; git -C $r commit -qm init; done; git init -q -b main client; echo '{"name":"client"}' > client/package.json; git -C client add -A; git -C client commit -qm init`
Enter
Type "clear"
Enter
Show

Type "tdm new my-feature api shared client"
Enter
Sleep 3s
Type "cat ~/workspaces/my-feature/go.work"
Enter
Sleep 3s
Type "clear"
Enter

Type "tdm ui"
Enter
Sleep 2s
Type "g"
Sleep 2s
Type "?"
Sleep 2s
Type "?"
Type "q"
Sleep 1s
