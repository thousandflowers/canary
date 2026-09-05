# canary installer tape — render with:  vhs install.tape
# produces assets/install.gif
# https://github.com/charmbracelet/vhs
#
# On camera: the one-liner from the README, and everything it draws — the
# platform it found, the bird singing through each step, the step log, and the
# binary it just installed drawing itself at the end.
#
# HOME is a throwaway directory with a short name. Short because the installer
# names the rc and the settings file it wired, and a 60-character temp path
# wraps those lines in half; throwaway because recording this must not wire the
# rc of the machine doing the recording.
#
# ~/.claude is created but left empty, which is also what a machine that has
# never run canary looks like.
#
# Every Type line is a backtick string. vhs cannot parse an escaped double
# quote inside a "..." string, which is easy to write and impossible to notice
# until the render fails.

Output assets/install.gif

Set Shell "bash"
# 103 columns at this size, which is what the longest line needs: the one-liner
# itself. Nothing else on screen comes close once HOME is short.
Set FontSize 20
Set Width 1240
Set Height 640
Set Padding 30

Hide
Type `export HOME=$(mktemp -d /tmp/cn.XX) && mkdir -p "$HOME/.claude"`
Enter
Type `export PS1='$ ' && clear`
Enter
Sleep 2s
Show

Type `curl -fsSL https://raw.githubusercontent.com/thousandflowers/canary/main/install.sh | sh`
Enter
Sleep 14s
