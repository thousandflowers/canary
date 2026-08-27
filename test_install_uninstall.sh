#!/usr/bin/env bash
# Self-check for install.sh / uninstall.sh. No framework, just asserts.
#   bash test_install_uninstall.sh   # exits non-zero on any failure
#
# Every run happens inside a throwaway $HOME, so this never touches the real
# ~/.zshrc, ~/.canary or ~/.claude/settings.json.
#
# SC2016: the rc fixtures below must contain a literal `$HOME`, exactly as
# install.sh writes it — expanding it here would test the wrong string.
# SC2012: `ls -l | cut` is the portable way to read a file's mode; stat(1) takes
# different flags on macOS and GNU.
# shellcheck disable=SC2016,SC2012

set -u
HERE=$(cd "$(dirname "$0")" && pwd)
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

fails=0
assert_eq()  { [ "$2" = "$3" ] && return; echo "FAIL [$1]: expected '$3', got '$2'"; fails=$((fails+1)); }
assert_has() { case "$2" in *"$3"*) ;; *) echo "FAIL [$1]: expected to contain '$3'"; echo "  got: $(printf '%s' "$2" | tr '\n' '|')"; fails=$((fails+1));; esac; }
assert_no()  { case "$2" in *"$3"*) echo "FAIL [$1]: should NOT contain '$3'"; fails=$((fails+1));; esac; }

# a fresh fake home, printed on stdout
newhome() { h="$TMP/h$RANDOM$RANDOM"; mkdir -p "$h"; printf '%s' "$h"; }
install_into()   { env HOME="$1" SHELL=/bin/bash CLAUDE_CONFIG_DIR="$1/.claude" sh "$HERE/install.sh"   >/dev/null 2>&1; }
uninstall_from() { env HOME="$1" SHELL=/bin/bash CLAUDE_CONFIG_DIR="$1/.claude" sh "$HERE/uninstall.sh" >/dev/null 2>&1; }

# --- 1. install drops the files and wires the rc -----------------------------
H=$(newhome); printf 'export PREEXISTING=1\n' > "$H/.bashrc"
install_into "$H"
[ -f "$H/.canary/canary.sh" ]            || { echo "FAIL [install-canary-sh]: not installed"; fails=$((fails+1)); }
[ -f "$H/.canary/canary-statusline.sh" ] || { echo "FAIL [install-statusline]: not installed"; fails=$((fails+1)); }
[ -f "$H/.canary/phrases/en/states/dead.txt" ] || { echo "FAIL [install-phrases]: corpus not installed"; fails=$((fails+1)); }
assert_has "install-rc-line"  "$(cat "$H/.bashrc")" '.canary/canary.sh'
assert_has "install-rc-keeps" "$(cat "$H/.bashrc")" 'export PREEXISTING=1'

# the installed bird actually loads and runs
out=$(env HOME="$H" CANARY_NIGHT_MULT=100 CANARY_STATE_FILE="$H/.canary/st" \
      bash -c ". '$H/.canary/canary.sh'; trap - DEBUG; canary status" 2>&1)
assert_has "install-runs" "$out" '▐ O ▌>'

# and the installed statusline bird can actually reach the corpus and speak —
# without this the phrase system ships inert and nothing notices
out=$(env HOME="$H" COLUMNS=100 bash "$H/.canary/canary-statusline.sh" preview --state dead 2>&1)
assert_has "install-phrases-speak" "$out" "the canary is quiet."

# --- 2. install is idempotent: one source line, not two ----------------------
install_into "$H"
n=$(grep -c '\.canary/canary\.sh' "$H/.bashrc")
assert_eq "install-idempotent" "$n" "1"

# --- 3. uninstall cleans the rc and removes ~/.canary ------------------------
uninstall_from "$H"
assert_no  "uninstall-rc-clean" "$(cat "$H/.bashrc")" '.canary/canary.sh'
assert_has "uninstall-rc-keeps" "$(cat "$H/.bashrc")" 'export PREEXISTING=1'
[ -d "$H/.canary" ] && { echo "FAIL [uninstall-home]: ~/.canary survived"; fails=$((fails+1)); }

# --- 3b. REGRESSION: bash must load in BOTH login and non-login shells -------
# install.sh used to wire ~/.bash_profile whenever ~/.bashrc did not exist.
# A Linux terminal opens an interactive NON-login shell, which reads .bashrc and
# never .bash_profile — so canary was installed into a file that shell never
# reads and the bird simply never appeared. macOS has the mirror problem: its
# terminals open LOGIN shells, which read .bash_profile and never .bashrc.
# The fix wires .bashrc always and chains the login rc to it, so all four
# starting states have to work in both shell modes.
bash_loads() { # <home> <login|nonlogin> -> "yes"/"no"
  # Asserts the BIRD RENDERS, not merely that `canary` is defined. A weaker
  # "did it say command not found" check passes even when the prompt hook was
  # never wired, which is most of what could go wrong here.
  if [ "$2" = login ]; then set -- "$1" -l -i; else set -- "$1" -i; fi
  h=$1; shift
  out=$(printf 'canary score\nexit\n' | env HOME="$h" CANARY_NIGHT_MULT=100 /bin/bash "$@" 2>&1)
  case "$out" in
    *"command not found"*) echo no ;;
    *"▐ O ▌>"*)            echo yes ;;   # the fresh bird, drawn by the prompt hook
    *)                     echo no ;;
  esac
}

for start in none bashrc-only profile-only both; do
  H=$(newhome)
  case $start in
    bashrc-only)  printf 'export KEEP=1\n' > "$H/.bashrc" ;;
    profile-only) printf 'export KEEP=1\n' > "$H/.bash_profile" ;;
    both)         printf 'export KEEP=1\n' > "$H/.bashrc"; printf 'export KEEP=1\n' > "$H/.bash_profile" ;;
  esac
  install_into "$H"
  assert_eq "bash-$start-nonlogin" "$(bash_loads "$H" nonlogin)" "yes"
  assert_eq "bash-$start-login"    "$(bash_loads "$H" login)"    "yes"

  # uninstall takes back everything it added, including the chain line
  uninstall_from "$H"
  left=$(cat "$H"/.bashrc "$H"/.bash_profile 2>/dev/null | grep -c 'canary' || true)
  assert_eq "bash-$start-uninstall-clean" "$left" "0"
  assert_eq "bash-$start-after-uninstall" "$(bash_loads "$H" nonlogin)" "no"
done

# a .bashrc chain the USER wrote must survive uninstall — we only take back the
# line we added, identified by our own marker comment sitting beside it
H=$(newhome)
printf 'export KEEP=1\n[ -f "$HOME/.bashrc" ] && . "$HOME/.bashrc"\n' > "$H/.bash_profile"
printf 'export KEEP=1\n' > "$H/.bashrc"
install_into "$H"; uninstall_from "$H"
assert_has "user-chain-kept" "$(cat "$H/.bash_profile")" '. "$HOME/.bashrc"'
assert_no  "user-chain-no-canary" "$(cat "$H/.bash_profile")" 'canary'

# --- 4. REGRESSION: a symlinked rc must survive uninstall --------------------
# ~/.zshrc is very often a symlink into a dotfiles repo. uninstall.sh used to
# `mv` a temp file over it, which replaced the symlink with a regular file AND
# left the dotfiles copy still sourcing canary — a silent no-op uninstall that
# also detached the rc from the repo.
H=$(newhome); mkdir -p "$H/dotfiles"
printf 'export FOO=1\n# canary — fatigue bird\n[ -f "$HOME/.canary/canary.sh" ] && . "$HOME/.canary/canary.sh"\nexport BAR=2\n' > "$H/dotfiles/zshrc"
ln -s "$H/dotfiles/zshrc" "$H/.zshrc"
uninstall_from "$H"
[ -L "$H/.zshrc" ] || { echo "FAIL [symlink-survives]: uninstall replaced the symlink with a regular file"; fails=$((fails+1)); }
assert_no  "symlink-target-cleaned" "$(cat "$H/dotfiles/zshrc")" '.canary/canary.sh'
assert_has "symlink-target-keeps"   "$(cat "$H/dotfiles/zshrc")" 'export BAR=2'

# --- 5. REGRESSION: uninstall must not change the rc's mode ------------------
# the old `mv` handed the rc mktemp's 0600, silently tightening a 0644 dotfile.
H=$(newhome)
printf 'export A=1\n# canary — fatigue bird\n[ -f "$HOME/.canary/canary.sh" ] && . "$HOME/.canary/canary.sh"\n' > "$H/.zshrc"
chmod 644 "$H/.zshrc"
uninstall_from "$H"
mode=$(ls -l "$H/.zshrc" | cut -c1-10)
assert_eq "perms-preserved" "$mode" "-rw-r--r--"

# --- 6. an rc with nothing of ours is left byte-identical --------------------
H=$(newhome); printf 'export UNRELATED=1\n' > "$H/.bashrc"
before=$(cat "$H/.bashrc")
uninstall_from "$H"
assert_eq "untouched-rc" "$(cat "$H/.bashrc")" "$before"

# --- 7. uninstall is idempotent ----------------------------------------------
uninstall_from "$H"
assert_eq "uninstall-idempotent" "$(cat "$H/.bashrc")" "$before"

# --- 8. statusline wiring appends, never replaces (needs jq) -----------------
if command -v jq >/dev/null 2>&1; then
  H=$(newhome); mkdir -p "$H/.claude"
  printf '{"statusLine":{"type":"command","command":"bash /opt/caveman.sh"}}\n' > "$H/.claude/settings.json"
  install_into "$H"
  cmd=$(jq -r '.statusLine.command' "$H/.claude/settings.json")
  assert_has "statusline-keeps-existing" "$cmd" 'bash /opt/caveman.sh'
  assert_has "statusline-adds-canary"    "$cmd" 'canary-statusline.sh'

  # ...and re-running install doesn't add it twice
  install_into "$H"
  n=$(jq -r '.statusLine.command' "$H/.claude/settings.json" | grep -c 'canary-statusline' || true)
  assert_eq "statusline-idempotent" "$n" "1"

  # uninstall removes only our segment
  uninstall_from "$H"
  cmd=$(jq -r '.statusLine.command // ""' "$H/.claude/settings.json")
  assert_eq "statusline-unwired" "$cmd" "bash /opt/caveman.sh"

  # a settings.json that was ONLY canary loses the whole statusLine key
  H=$(newhome); mkdir -p "$H/.claude"; printf '{}\n' > "$H/.claude/settings.json"
  install_into "$H"; uninstall_from "$H"
  assert_eq "statusline-removed" "$(jq -r '.statusLine // "gone"' "$H/.claude/settings.json")" "gone"

  # malformed settings.json (JSONC) must never be clobbered
  H=$(newhome); mkdir -p "$H/.claude"
  printf '// a comment\n{"statusLine":{"command":"x"}}\n' > "$H/.claude/settings.json"
  before=$(cat "$H/.claude/settings.json")
  install_into "$H"
  assert_eq "statusline-jsonc-safe" "$(cat "$H/.claude/settings.json")" "$before"
else
  echo "skip — jq not installed, statusline wiring checks skipped"
fi

if [ "$fails" -eq 0 ]; then echo "ok — all install/uninstall checks passed"; else echo "$fails check(s) failed"; exit 1; fi
