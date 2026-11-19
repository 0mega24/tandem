# Sourced, hidden, by the tapes: a scratch HOME with three small repos, so a recording never
# reads real config or clones. api and shared are Go; client is JavaScript. The short fixed path
# keeps the paths in the output readable.
rm -rf /tmp/tandem-demo
mkdir /tmp/tandem-demo
export HOME=/tmp/tandem-demo XDG_CONFIG_DIRS=/nonexistent GIT_CONFIG_NOSYSTEM=1
export GIT_AUTHOR_NAME=demo GIT_AUTHOR_EMAIL=demo@example.invalid
export GIT_COMMITTER_NAME=demo GIT_COMMITTER_EMAIL=demo@example.invalid
git config --global init.defaultBranch main
mkdir -p ~/repos ~/origins ~/.config/tandem
# Only the generators of the demo repos, so a dry run fits on one screen.
printf '[generate]\nenabled = ["go", "js"]\n' > ~/.config/tandem/config.toml
for r in api shared client; do
  git init -q --bare ~/origins/$r.git
  git init -q ~/repos/$r
  case $r in
    client) echo '{"name": "client", "private": true}' > ~/repos/$r/package.json ;;
    *) printf 'module example.com/%s\n\ngo 1.22\n' $r > ~/repos/$r/go.mod ;;
  esac
  git -C ~/repos/$r add -A && git -C ~/repos/$r commit -qm init
  git -C ~/repos/$r remote add origin ~/origins/$r.git && git -C ~/repos/$r push -q -u origin main
done
cd ~
clear
