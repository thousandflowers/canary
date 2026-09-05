# canary installer tape — render with:
#
#   vhs install.tape
#   ffmpeg -ss 0.68 -t 1.40 -i install-raw.gif \
#     -vf "crop=500:66:22:214,split[a][b];[a]palettegen=stats_mode=diff[p];[b][p]paletteuse" \
#     -loop 0 assets/install.gif
#
# Two steps on purpose. What belongs in a README is the bird singing and the
# step it is singing about — 500x66 of it. The rest of an install is paths and
# a step log: true, useful in a terminal, and a wall of text in a page. vhs
# cannot crop, so the tape records the whole window and ffmpeg keeps the strip
# where the bird is (y 214) for the seconds where it is the only thing moving.
#
# Re-derive the numbers if the layout changes: the crop is the two rows the
# bird draws, and the window is from the first frame it appears to the last one
# before the step log lands underneath it.
#
# HOME is a throwaway directory, so recording this cannot wire the rc of the
# machine doing the recording. The Go caches are passed through from the real
# HOME: a cold cache would re-download every module and stretch the recording
# into a minute of nothing.
#
# Every Type line is a backtick string. vhs cannot parse an escaped double
# quote inside a "..." string, which is easy to write and impossible to notice
# until the render fails.

Output install-raw.gif

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
