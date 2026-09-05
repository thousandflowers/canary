# canary demo tape - render with:  vhs demo.tape
# produces assets/demo.gif
# https://github.com/charmbracelet/vhs
#
# The whole recording is `canary demo`, which draws the bird in place and
# clears the screen first. Nothing else is on camera: a recording of somebody
# typing commands into a shell shows the commands, not the canary.
#
# Every Type line is a backtick string. vhs cannot parse an escaped double
# quote inside a "..." string, which is easy to write and impossible to notice
# until the render fails.

Output assets/demo.gif

Set Shell "bash"
# Sized so the bird is legible at the width a README renders it, and COLUMNS
# is exported to match: fifty-two columns of bird and phrase, not eighty. The bird trims a phrase to the cells it believes it
# has, and COLUMNS is the only thing it can believe: a window narrower than the
# assumption wraps the row instead of trimming it, and a wider one is dead
# space.
Set FontSize 36
Set Width 1200
Set Height 240
Set Padding 24

# build off camera; the demo itself clears the screen, so the command never
# appears in a recorded frame
Hide
Type `go build -o /tmp/canary ./cmd/canary`
Enter
Sleep 5s
Type `export COLUMNS=52`
Enter
Type `/tmp/canary demo`
Enter
Sleep 500ms
Show

# fresh speaks, then moves; chirpy, tired and worn the same; dead says its one
# line and then stops.
#
# Deliberately a little SHORTER than the demo: the recording has to end while
# the dead bird is still sitting in its silence, not after the shell prompt has
# come back underneath it.
Sleep 21500ms
