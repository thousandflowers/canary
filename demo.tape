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

# fast-forward ~40 min -> chirpy
Type "CANARY_START_TIME=$(( $(date +%s) - 40*60 )); CANARY_PROMPT_COUNT=12"
Enter
Sleep 1500ms

# ~80 min, 45 commands -> tired
Type "CANARY_START_TIME=$(( $(date +%s) - 80*60 )); CANARY_PROMPT_COUNT=45"
Enter
Sleep 1500ms

# ~110 min, 70 commands -> worn
Type "CANARY_START_TIME=$(( $(date +%s) - 110*60 )); CANARY_PROMPT_COUNT=70"
Enter
Sleep 1500ms

# ~140 min, 90 commands -> dead
Type "CANARY_START_TIME=$(( $(date +%s) - 140*60 )); CANARY_PROMPT_COUNT=90"
Enter
Sleep 2500ms

# fresh again
Type "CANARY_RESET=1"
Enter
Sleep 2000ms
