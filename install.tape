# canary installer tape — render with:  vhs install.tape
# produces assets/install.gif
# https://github.com/charmbracelet/vhs
#
# What is on camera is one command, `sh ./install.sh`, and what it draws: the
# platform it found, the bird singing through each step, and the binary it just
# installed drawing itself at the end.
#
# HOME is a throwaway directory, so recording this cannot wire the rc of the
# machine doing the recording. The Go caches are passed through from the real
# HOME: a cold cache would re-download every module and the recording would be
# a minute of singing with nothing to show for it.
#
# Every Type line is a backtick string. vhs cannot parse an escaped double
# quote inside a "..." string, which is easy to write and impossible to notice
# until the render fails.

Output assets/install.gif

Set Shell "bash"
Set FontSize 20
Set Width 1240
Set Height 600
Set Padding 24

Hide
Type `export GOPATH=$(go env GOPATH) GOCACHE=$(go env GOCACHE) GOMODCACHE=$(go env GOMODCACHE)`
Enter
# A short HOME on purpose: the installer names the rc it wired, and the
# default temp directory is long enough to wrap that line twice on camera.
Type `export HOME=$(mktemp -d /tmp/canary.XXXX) && mkdir -p "$HOME/.claude" && printf '{}\n' > "$HOME/.claude/settings.json"`
Enter
# Warm the build cache off camera. Otherwise the first thing on screen is go
# downloading two modules, which is a recording of a package manager.
Type `go build -o /dev/null ./cmd/canary`
Enter
Sleep 20s
Type `export PS1='$ ' && clear`
Enter
Sleep 2s
Show

Type `sh ./install.sh`
Enter
Sleep 22s
