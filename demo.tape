# canary demo tape — render with:  vhs demo.tape
# produces assets/demo.gif (the bird wilting as a "session" wears on)
# https://github.com/charmbracelet/vhs

Output assets/demo.gif

Set Shell "bash"
Set FontSize 22
Set Width 760
Set Height 360
Set Padding 24
Set TypingSpeed 60ms

# load canary in a clean prompt, quietly
Hide
Type "source ./canary.sh; export CANARY_SHOW_SCORE=1; PS1='$ '"
Enter
Type "clear"
Enter
Show

# fresh
Type "echo 'just sat down'"
Enter
Sleep 1500ms

# fast-forward ~60 active min -> chirpy
Type "CANARY_ACTIVE_SECONDS=$((60*60)); CANARY_PROMPT_COUNT=25"
Enter
Sleep 1500ms

# ~120 active min, 45 commands -> tired
Type "CANARY_ACTIVE_SECONDS=$((120*60)); CANARY_PROMPT_COUNT=45"
Enter
Sleep 1500ms

# ~180 active min, 60 commands -> worn
Type "CANARY_ACTIVE_SECONDS=$((180*60)); CANARY_PROMPT_COUNT=60"
Enter
Sleep 1500ms

# ~240 active min, 90 commands -> dead
Type "CANARY_ACTIVE_SECONDS=$((240*60)); CANARY_PROMPT_COUNT=90"
Enter
Sleep 2500ms

# fresh again
Type "CANARY_RESET=1"
Enter
Sleep 2000ms
