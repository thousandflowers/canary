# canary demo tape - render with:  vhs demo.tape
# produces assets/demo.gif (the bird wilting as a "session" wears on)
# https://github.com/charmbracelet/vhs
#
# The session is fast-forwarded by writing the state file the bird reads, not by
# waiting four hours. CANARY_STATE_FILE points at a throwaway path on purpose:
# recording the demo must not reset the recorder's own bird.

Output assets/demo.gif

Set Shell "bash"
Set FontSize 22
Set Width 760
Set Height 360
Set Padding 24
Set TypingSpeed 60ms

# build, load the hook, and define the fast-forward helper — all off camera
Hide
Type "go build -o /tmp/canary ./cmd/canary"
Enter
Type "export CANARY_STATE_FILE=$(mktemp) CANARY_SHOW_SCORE=1 CANARY_NIGHT_MULT=100 PS1='$ '"
Enter
Type "fastforward() { printf 'prompt_count=%s\nlen_sum=%s\nactive_seconds=%s\nlast_active=0\n' \"$2\" \"$((20 * $2))\" \"$((60 * $1))\" > $CANARY_STATE_FILE; }"
Enter
Type 'eval "$(/tmp/canary init bash)"'
Enter
Type "clear"
Enter
Show

# fresh
Type "echo 'just sat down'"
Enter
Sleep 1500ms

# ~60 active minutes, 25 commands -> chirpy
Type "fastforward 60 25"
Enter
Sleep 1500ms

# ~120 minutes, 45 commands -> tired
Type "fastforward 120 45"
Enter
Sleep 1500ms

# ~180 minutes, 60 commands -> worn
Type "fastforward 180 60"
Enter
Sleep 1500ms

# ~240 minutes, 90 commands -> dead
Type "fastforward 240 90"
Enter
Sleep 2500ms

# fresh again
Type "/tmp/canary reset"
Enter
Sleep 2000ms
