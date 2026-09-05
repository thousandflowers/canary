# canary installer tape — render with:
#
#   vhs install.tape
#   ffmpeg -ss 0.85 -t 5.0 -i install-raw.gif \
#     -vf "crop=1100:140:0:8,split[a][b];[a]palettegen=stats_mode=diff[p];[b][p]paletteuse" \
#     -loop 0 assets/install.gif
#
# What goes in the README is the bird and the step it is singing about. Not the
# install log: temp paths and wrapped lines are true, useful in a terminal, and
# ugly in a page.
#
# `sh ./install.sh` rather than the one-liner, because the command never makes
# it into frame: it scrolls off before the bird starts, and the clone path is
# the one the section around the gif is about.
#
# So the window is three rows tall. Everything the installer prints before the
# animation scrolls off the top on its own, the two rows the bird redraws are
# all that is left in frame, and nothing has to be cropped afterwards — a crop
# is what clipped the art the first time. ffmpeg only trims the recording to
# the seconds where the bird is singing, before the step log lands.
#
# Wide enough for the longest bird row — the art, the notes and the longest
# step label — because install.sh rewinds exactly two rows and a wrapped row
# would leave half a bird on screen forever.
#
# HOME is a throwaway directory: recording this must not wire the rc of the
# machine doing the recording.
#
# Every Type line is a backtick string. vhs cannot parse an escaped double
# quote inside a "..." string, which is easy to write and impossible to notice
# until the render fails.

Output install-raw.gif

Set Shell "bash"
Set FontSize 36
Set Width 1100
Set Height 190
Set Padding 20

Hide
Type `export HOME=$(mktemp -d /tmp/cXX) && mkdir -p "$HOME/.claude"`
Enter
Type `export PS1='$ ' && clear`
Enter
Sleep 2s
Show

Type `sh ./install.sh`
Enter
Sleep 14s
