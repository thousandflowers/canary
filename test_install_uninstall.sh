#!/usr/bin/env bash
# Self-check for install.sh / uninstall.sh. No framework, just asserts.
#   bash test_install_uninstall.sh   # exits non-zero on any failure
#
# Every run happens inside a throwaway $HOME, so this never touches the real
# ~/.zshrc, ~/.canary or ~/.claude/settings.json.
#
# What the Go suites cannot cover: the installer is still shell, because a
# `curl | sh` bootstrap cannot be written in the language it is bootstrapping.
#
# SC2016: the rc fixtures below must contain a literal `$HOME`, exactly as
# install.sh writes it — expanding it here would test the wrong string.
# SC2012: `ls -l | cut` is the portable way to read a file's mode; stat(1) takes
# different flags on macOS and GNU.
# shellcheck disable=SC2016,SC2012

set -u
HERE=$(cd "$(dirname "$0")" && pwd)
TMP=$(mktemp -d)
# chmod first: a Go build inside a fake $HOME would otherwise leave a read-only
# module cache behind that the cleanup cannot clear.
trap 'chmod -R u+w "$TMP" 2>/dev/null; rm -rf "$TMP"' EXIT

fails=0
assert_eq()  { [ "$2" = "$3" ] && return; echo "FAIL [$1]: expected '$3', got '$2'"; fails=$((fails+1)); }
assert_has() { case "$2" in *"$3"*) ;; *) echo "FAIL [$1]: expected to contain '$3'"; echo "  got: $(printf '%s' "$2" | tr '\n' '|')"; fails=$((fails+1));; esac; }
assert_no()  { case "$2" in *"$3"*) echo "FAIL [$1]: should NOT contain '$3'"; fails=$((fails+1));; esac; }

# a fresh fake home, printed on stdout
newhome() { h="$TMP/h$RANDOM$RANDOM"; mkdir -p "$h"; printf '%s' "$h"; }
# The Go caches are passed through explicitly. They live under $HOME by default,
# and a fake $HOME per test would re-download the module cache for every install
# — a test suite that needs the network is not a test suite.
install_into()   { env HOME="$1" SHELL=/bin/bash CLAUDE_CONFIG_DIR="$1/.claude" \
                       GOPATH="$GOPATH_REAL" GOCACHE="$GOCACHE_REAL" GOMODCACHE="$GOMODCACHE_REAL" \
                       sh "$HERE/install.sh"   >/dev/null 2>&1; }
# same, with flags for the installer: install_flags <home> [flag...]
install_flags()  { h="$1"; shift; env HOME="$h" SHELL=/bin/bash CLAUDE_CONFIG_DIR="$h/.claude" \
                       GOPATH="$GOPATH_REAL" GOCACHE="$GOCACHE_REAL" GOMODCACHE="$GOMODCACHE_REAL" \
                       sh "$HERE/install.sh" "$@" >/dev/null 2>&1; }
uninstall_from() { env HOME="$1" SHELL=/bin/bash CLAUDE_CONFIG_DIR="$1/.claude" sh "$HERE/uninstall.sh" >/dev/null 2>&1; }
bin_of() { printf '%s' "$1/.local/bin/canary"; }

# The installer builds from source when it finds go.mod and Go — which is the
# path CI and contributors take. Without Go there is nothing to install but a
# release download, and a test that hits the network is not a test.
if ! command -v go >/dev/null 2>&1; then
  echo "skip — Go not installed, the installer has nothing local to build"
  exit 0
fi
GOPATH_REAL=$(go env GOPATH)
GOCACHE_REAL=$(go env GOCACHE)
GOMODCACHE_REAL=$(go env GOMODCACHE)

# --- 1. install puts the binary somewhere runnable and wires the rc ----------
H=$(newhome); printf 'export PREEXISTING=1\n' > "$H/.bashrc"
install_into "$H"
[ -x "$(bin_of "$H")" ] || { echo "FAIL [install-binary]: no binary at $(bin_of "$H")"; fails=$((fails+1)); }
assert_has "install-rc-line"  "$(cat "$H/.bashrc")" 'init bash'
assert_has "install-rc-path"  "$(cat "$H/.bashrc")" '.local/bin'
assert_has "install-rc-keeps" "$(cat "$H/.bashrc")" 'export PREEXISTING=1'

# the installed bird actually runs
out=$(env HOME="$H" CANARY_NIGHT_MULT=100 "$(bin_of "$H")" status 2>&1)
assert_has "install-runs" "$out" '▐ O ▌>'

# and it can speak with nothing but itself: the corpus is compiled in, so there
# is no ~/.canary/phrases for the packaging to forget. Shipping mute was the
# v0.7.0 bug this replaces.
[ -d "$H/.canary/phrases" ] && { echo "FAIL [install-no-corpus-copy]: the corpus was copied to disk"; fails=$((fails+1)); }
out=$(env HOME="$H" COLUMNS=100 "$(bin_of "$H")" preview --state dead 2>&1)
assert_has "install-phrases-speak" "$out" "the canary is quiet."

# --- 2. install is idempotent: one hook line, not two ------------------------
install_into "$H"
n=$(grep -c 'init bash' "$H/.bashrc")
assert_eq "install-idempotent" "$n" "1"

# --- 3. uninstall cleans the rc, ~/.canary and the binary --------------------
env HOME="$H" "$(bin_of "$H")" record -- "some work" >/dev/null 2>&1
uninstall_from "$H"
assert_no  "uninstall-rc-clean" "$(cat "$H/.bashrc")" 'init bash'
assert_no  "uninstall-rc-path"  "$(cat "$H/.bashrc")" '.local/bin'
assert_has "uninstall-rc-keeps" "$(cat "$H/.bashrc")" 'export PREEXISTING=1'
[ -d "$H/.canary" ] && { echo "FAIL [uninstall-home]: ~/.canary survived"; fails=$((fails+1)); }
[ -f "$(bin_of "$H")" ] && { echo "FAIL [uninstall-binary]: the binary survived"; fails=$((fails+1)); }

# --- 3b. REGRESSION: bash must load in BOTH login and non-login shells -------
# install.sh used to wire ~/.bash_profile whenever ~/.bashrc did not exist.
# A Linux terminal opens an interactive NON-login shell, which reads .bashrc and
# never .bash_profile — so canary was installed into a file that shell never
# reads and the bird simply never appeared. macOS has the mirror problem: its
# terminals open LOGIN shells, which read .bash_profile and never .bashrc.
# The fix wires .bashrc always and chains the login rc to it, so all four
# starting states have to work in both shell modes.
bash_loads() { # <home> <login|nonlogin> -> "yes"/"no"
  # Asserts the BIRD RENDERS, not merely that the binary is on PATH. A weaker
  # "did it say command not found" check passes even when the prompt hook was
  # never wired, which is most of what could go wrong here.
  if [ "$2" = login ]; then set -- "$1" -l -i; else set -- "$1" -i; fi
  h=$1; shift
  out=$(printf 'true\nexit\n' | env HOME="$h" CANARY_NIGHT_MULT=100 /bin/bash "$@" 2>&1)
  case "$out" in
    *"▐ O ▌>"*) echo yes ;;   # the fresh bird, drawn by the prompt hook
    *)          echo no ;;
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

# --- 3c. an rc left by the pre-1.0 shell version is cleaned too --------------
# Upgrading from 0.x leaves `. ~/.canary/canary.sh` behind. If uninstall stopped
# recognising it, that line would keep sourcing a script that no longer exists.
H=$(newhome)
printf 'export KEEP=1\n# canary — fatigue bird\n[ -f "$HOME/.canary/canary.sh" ] && . "$HOME/.canary/canary.sh"\n' > "$H/.zshrc"
uninstall_from "$H"
assert_no  "legacy-rc-cleaned" "$(cat "$H/.zshrc")" 'canary.sh'
assert_has "legacy-rc-keeps"   "$(cat "$H/.zshrc")" 'export KEEP=1'

# --- 4. REGRESSION: a symlinked rc must survive uninstall --------------------
# ~/.zshrc is very often a symlink into a dotfiles repo. uninstall.sh used to
# `mv` a temp file over it, which replaced the symlink with a regular file AND
# left the dotfiles copy still sourcing canary — a silent no-op uninstall that
# also detached the rc from the repo.
H=$(newhome); mkdir -p "$H/dotfiles"
printf 'export FOO=1\n# canary — fatigue bird\neval "$("%s/.local/bin/canary" init zsh)"\nexport BAR=2\n' "$H" > "$H/dotfiles/zshrc"
ln -s "$H/dotfiles/zshrc" "$H/.zshrc"
uninstall_from "$H"
[ -L "$H/.zshrc" ] || { echo "FAIL [symlink-survives]: uninstall replaced the symlink with a regular file"; fails=$((fails+1)); }
assert_no  "symlink-target-cleaned" "$(cat "$H/dotfiles/zshrc")" 'init zsh'
assert_has "symlink-target-keeps"   "$(cat "$H/dotfiles/zshrc")" 'export BAR=2'

# --- 5. REGRESSION: uninstall must not change the rc's mode ------------------
# the old `mv` handed the rc mktemp's 0600, silently tightening a 0644 dotfile.
H=$(newhome)
printf 'export A=1\n# canary — fatigue bird\neval "$("%s/.local/bin/canary" init zsh)"\n' "$H" > "$H/.zshrc"
chmod 644 "$H/.zshrc"
uninstall_from "$H"
mode=$(ls -l "$H/.zshrc" | cut -c1-10)
assert_eq "perms-preserved" "$mode" "-rw-r--r--"

# --- 6. an rc with nothing of ours is left byte-identical --------------------
H=$(newhome); printf 'export UNRELATED=1\n' > "$H/.bashrc"
before=$(cat "$H/.bashrc")
uninstall_from "$H"
assert_eq "untouched-rc" "$(cat "$H/.bashrc")" "$before"

# --- 6b. someone else's PATH line is not ours to remove ----------------------
# The PATH export is matched whole and literal for exactly this reason.
H=$(newhome); printf 'export PATH="/opt/other/bin:$PATH"\n# canary — fatigue bird\neval "$("%s/.local/bin/canary" init zsh)"\n' "$H" > "$H/.zshrc"
uninstall_from "$H"
assert_has "foreign-path-kept" "$(cat "$H/.zshrc")" '/opt/other/bin'

# --- 7. uninstall is idempotent ----------------------------------------------
uninstall_from "$H"
assert_has "uninstall-idempotent" "$(cat "$H/.zshrc")" '/opt/other/bin'

# --- 8. statusline wiring appends, never replaces ----------------------------
# No jq in the installer any more — the binary edits the JSON itself. jq is used
# here only to READ the result back.
if command -v jq >/dev/null 2>&1; then
  H=$(newhome); mkdir -p "$H/.claude"
  printf '{"statusLine":{"type":"command","command":"bash /opt/caveman.sh"}}\n' > "$H/.claude/settings.json"
  install_into "$H"
  cmd=$(jq -r '.statusLine.command' "$H/.claude/settings.json")
  assert_has "statusline-keeps-existing" "$cmd" 'bash /opt/caveman.sh'
  assert_has "statusline-adds-canary"    "$cmd" 'canary" statusline'

  # ...and re-running install doesn't add it twice
  install_into "$H"
  n=$(jq -r '.statusLine.command' "$H/.claude/settings.json" | grep -c 'statusline' || true)
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

# --- 8b. install then uninstall leaves the rc byte-identical -----------------
# The README promises the uninstall takes back what the install added. A blank
# line left behind is still a fingerprint, and four rounds of install/uninstall
# used to grow four of them.
H=$(newhome); printf 'export PREEXISTING=1\n\n# my own spacing above\n\nexport LATER=2\n' > "$H/.bashrc"
before=$(cat "$H/.bashrc")
install_into "$H"; uninstall_from "$H"
assert_eq "roundtrip-byte-identical" "$(cat "$H/.bashrc")" "$before"

# and it stays identical after several rounds
install_into "$H"; uninstall_from "$H"
install_into "$H"; uninstall_from "$H"
assert_eq "roundtrip-three-times" "$(cat "$H/.bashrc")" "$before"

# --- 8c. an rc it cannot write is a failure, not a success ------------------
# The installer used to print "added the hook", sign off with "open a new shell
# to meet it", and change nothing: the bird never appeared and there was no way
# to find out why.
if [ "$(id -u)" != 0 ]; then   # root writes read-only files, and CI sometimes is root
  H=$(newhome); printf 'export PREEXISTING=1\n' > "$H/.bashrc"; chmod 444 "$H/.bashrc"
  if install_into "$H"; then
    echo "FAIL [readonly-rc-exit]: the installer reported success"; fails=$((fails+1))
  fi
  assert_eq "readonly-rc-untouched" "$(cat "$H/.bashrc")" "export PREEXISTING=1"
  out=$(env HOME="$H" SHELL=/bin/bash CLAUDE_CONFIG_DIR="$H/.claude" sh "$HERE/install.sh" 2>&1 || true)
  assert_has "readonly-rc-says-why" "$out" "cannot write"
  chmod 644 "$H/.bashrc"
fi

# --- 9. --claude-only wires Claude Code and leaves the rc alone --------------
# Two birds, one binary, and some people want only the one in Claude Code. The
# rc has to come out byte-identical: an installer that edits your shell anyway
# is the reason this flag exists.
H=$(newhome); printf 'export PREEXISTING=1\n' > "$H/.bashrc"; mkdir -p "$H/.claude"
printf '{}\n' > "$H/.claude/settings.json"
install_flags "$H" --claude-only
[ -x "$(bin_of "$H")" ] || { echo "FAIL [claude-only-binary]: no binary at $(bin_of "$H")"; fails=$((fails+1)); }
assert_eq "claude-only-rc-untouched" "$(cat "$H/.bashrc")" "export PREEXISTING=1"
assert_has "claude-only-statusline"  "$(cat "$H/.claude/settings.json")" 'statusline'

# and the env var, which is how the curl path asks for the same thing
H=$(newhome); printf 'export PREEXISTING=1\n' > "$H/.bashrc"; mkdir -p "$H/.claude"
printf '{}\n' > "$H/.claude/settings.json"
CANARY_CLAUDE_ONLY=1; export CANARY_CLAUDE_ONLY
install_into "$H"
unset CANARY_CLAUDE_ONLY
assert_eq "claude-only-env-rc-untouched" "$(cat "$H/.bashrc")" "export PREEXISTING=1"
assert_has "claude-only-env-statusline"  "$(cat "$H/.claude/settings.json")" 'statusline'

# an unknown flag is a usage error, not a silent full install
H=$(newhome); printf 'export PREEXISTING=1\n' > "$H/.bashrc"
if install_flags "$H" --nope; then
  echo "FAIL [claude-only-bad-flag]: an unknown flag exited 0"; fails=$((fails+1))
fi
assert_eq "claude-only-bad-flag-rc" "$(cat "$H/.bashrc")" "export PREEXISTING=1"

# --- the animation stays out of anything that is not a terminal -------------
# A pipe, a CI log or a redirect must get words, not cursor movement. This is
# the regression that turns a build log into a wall of ^[[2A.
H=$(newhome)
out=$(env HOME="$H" SHELL=/bin/bash CLAUDE_CONFIG_DIR="$H/.claude" \
      GOPATH="$GOPATH_REAL" GOCACHE="$GOCACHE_REAL" GOMODCACHE="$GOMODCACHE_REAL" \
      sh "$HERE/install.sh" 2>&1)
case $out in
  *"$(printf '\033')"*) echo "FAIL [install-plain-no-ansi]: escape codes with no terminal attached"; fails=$((fails+1)) ;;
esac
assert_has "install-plain-speaks" "$out" "canary:"

# --- the installer draws the same bird, and sings the same notes ------------
# install.sh cannot ask the binary for either: on the curl path there is no
# binary yet when the first frame goes up. So both are written twice, and this
# is what keeps the copies honest.
fresh_art=$(sed -n '/^func ArtFor/,/^}/p' "$HERE/internal/render/render.go" \
            | sed -n '/default:/,/}/p' | grep -o '"[^"]*"' | tr -d '"')
installer=$(cat "$HERE/install.sh")
for glyph in $fresh_art; do
  case $installer in
    *"$glyph"*) ;;
    *) echo "FAIL [install-art-parity]: ArtFor draws '$glyph' for fresh, install.sh does not"; fails=$((fails+1)) ;;
  esac
done

# --- 5. a build from an export without .git says the manifest's version -----
# Claude Code keeps its copy of the plugin without .git, so Go cannot read the
# tag and the binary would say `dev`. The installer stamps the version from
# .claude-plugin/plugin.json there, which release CI holds equal to the tag.
export_dir="$TMP/export"; mkdir -p "$export_dir"
(cd "$HERE" && tar --exclude=.git --exclude=dist -cf - .) | tar -xf - -C "$export_dir"
manifest_v=$(sed -n 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$HERE/.claude-plugin/plugin.json")
h=$(newhome); mkdir -p "$h/.claude"; echo '{}' > "$h/.claude/settings.json"
env HOME="$h" SHELL=/bin/bash CLAUDE_CONFIG_DIR="$h/.claude" \
    GOPATH="$GOPATH_REAL" GOCACHE="$GOCACHE_REAL" GOMODCACHE="$GOMODCACHE_REAL" \
    sh "$export_dir/install.sh" --claude-only >/dev/null 2>&1
assert_eq "export-build-version" "$("$(bin_of "$h")" version)" "canary $manifest_v"

# `sh install.sh` from inside the checkout, no ./ in front, must still build:
# $0 has no slash then, and the source lookup used to give up and download.
h=$(newhome); mkdir -p "$h/.claude"; echo '{}' > "$h/.claude/settings.json"
bare=$(cd "$HERE" && env HOME="$h" SHELL=/bin/bash CLAUDE_CONFIG_DIR="$h/.claude" \
    GOPATH="$GOPATH_REAL" GOCACHE="$GOCACHE_REAL" GOMODCACHE="$GOMODCACHE_REAL" \
    sh install.sh --claude-only 2>&1)
assert_has "bare-name-builds" "$bare" "building from source"

# A build that fails must say so and exit non-zero. It used to return 0 with
# the old binary still in place and "status line wired" printed underneath:
# the installer signing off on work it had not done.
bad_dir="$TMP/export-bad"; mkdir -p "$bad_dir"
(cd "$export_dir" && tar -cf - .) | tar -xf - -C "$bad_dir"
echo 'this is not go' >> "$bad_dir/cmd/canary/main.go"
h=$(newhome); mkdir -p "$h/.claude"; echo '{}' > "$h/.claude/settings.json"
if out=$(env HOME="$h" SHELL=/bin/bash CLAUDE_CONFIG_DIR="$h/.claude" \
    GOPATH="$GOPATH_REAL" GOCACHE="$GOCACHE_REAL" GOMODCACHE="$GOMODCACHE_REAL" \
    sh "$bad_dir/install.sh" --claude-only 2>&1); then
  echo "FAIL [bad-build-exit]: a failed build exited 0"; fails=$((fails+1))
fi
assert_has "bad-build-says-so" "$out" "the build failed"
assert_no  "bad-build-no-wire" "$out" "status line wired"
[ -e "$(bin_of "$h")" ] && { echo "FAIL [bad-build-no-binary]: a failed build left a binary behind"; fails=$((fails+1)); }
[ -z "$(ls "$h/.local/bin" 2>/dev/null)" ] || { echo "FAIL [bad-build-no-tmp]: a failed build left a staged file behind"; fails=$((fails+1)); }

# --- 6. the plugin hook: install once, keep its own binary current, touch nothing else
hook="$export_dir/hooks/session-start.sh"
# PATH with Go but no canary on it, the way a Claude Code launched from the Dock
# sees it. Go is linked into a directory of its own: on a Homebrew machine it
# shares one with an installed canary, which would make the cold start warm.
mkdir -p "$TMP/gobin"; ln -s "$(command -v go)" "$TMP/gobin/go"
bare_path="/usr/bin:/bin:$TMP/gobin"
run_hook() { env HOME="$1" PATH="$2" CLAUDE_PLUGIN_ROOT="$export_dir" CLAUDE_CONFIG_DIR="$1/.claude" \
                 GOPATH="$GOPATH_REAL" GOCACHE="$GOCACHE_REAL" GOMODCACHE="$GOMODCACHE_REAL" \
                 sh "$hook" 2>&1; }

# The hook fetches the release binary rather than building one, and a test
# that hits the network is not a test. So: no curl on PATH, which is the one
# failure the fetch has, and the hook must say so rather than stay silent.
mkdir -p "$TMP/nocurl"; ln -s "$(command -v go)" "$TMP/nocurl/go"
for tool in sh sed mkdir cat printf mv rm cp chmod dirname basename mktemp tar uname; do
  p=$(command -v "$tool") && ln -sf "$p" "$TMP/nocurl/$tool"
done
h=$(newhome); mkdir -p "$h/.claude"; echo '{}' > "$h/.claude/settings.json"
out=$(run_hook "$h" "$TMP/nocurl")
assert_has "hook-cold-says-so" "$out" "install failed"
[ -e "$(bin_of "$h")" ] && { echo "FAIL [hook-cold-half]: a failed fetch left a binary"; fails=$((fails+1)); }
[ -e "$h/.canary/plugin-version" ] && { echo "FAIL [hook-cold-stamp]: a failed install was stamped"; fails=$((fails+1)); }
[ -f "$h/.bashrc" ] && { echo "FAIL [hook-cold-rc]: the hook wired a shell rc"; fails=$((fails+1)); }

# The fetch, without the network: a curl on PATH that hands back a release
# archive made from the export build. The installer's own download path runs
# unchanged — the URL it asks for is simply answered from disk.
mkdir -p "$TMP/fakecurl" "$TMP/rel"
(cd "$export_dir" && go build "-ldflags=-X main.version=$manifest_v" -o "$TMP/rel/canary" ./cmd/canary) # stamped, as goreleaser does
(cd "$TMP/rel" && tar -czf "$TMP/release.tar.gz" canary)
printf '#!/bin/sh\ncat "%s"\n' "$TMP/release.tar.gz" > "$TMP/fakecurl/curl"; chmod +x "$TMP/fakecurl/curl"
fetch_path="$TMP/fakecurl:$bare_path"

# A binary already at the installer's path, put there by the one-liner or a
# clone, is the plugin's to keep current: adopted, wired and stamped, with the
# rc left alone. (Replaced here because the stamp is missing.)
h=$(newhome); mkdir -p "$h/.claude" "$h/.local/bin"; echo '{}' > "$h/.claude/settings.json"
printf '#!/bin/sh\necho canary 0.9.0\n' > "$(bin_of "$h")"; chmod +x "$(bin_of "$h")"
out=$(run_hook "$h" "$fetch_path")
assert_eq  "hook-adopt-silent"  "$out" ""
assert_eq  "hook-adopt-version" "$("$(bin_of "$h")" version)" "canary $manifest_v"
assert_has "hook-adopt-wired"   "$(cat "$h/.claude/settings.json")" "statusline"
assert_eq  "hook-adopt-stamp"   "$(cat "$h/.canary/plugin-version")" "$manifest_v"
[ -f "$h/.bashrc" ] && { echo "FAIL [hook-adopt-rc]: the hook wired a shell rc"; fails=$((fails+1)); }

# warm: the stamp matches → the binary is not touched. A sentinel in its place proves it.
printf '#!/bin/sh\necho canary sentinel\n' > "$(bin_of "$h")"; chmod +x "$(bin_of "$h")"
before=$(cat "$h/.claude/settings.json")
out=$(run_hook "$h" "$bare_path")
assert_eq "hook-warm-silent"    "$out" ""
assert_eq "hook-warm-untouched" "$("$(bin_of "$h")" version)" "canary sentinel"
assert_eq "hook-warm-settings"  "$(cat "$h/.claude/settings.json")" "$before"

# a plugin update: the stamp is behind the manifest → its own binary is replaced
echo "0.0.1" > "$h/.canary/plugin-version"
out=$(run_hook "$h" "$fetch_path")
assert_eq "hook-upgrade-silent"  "$out" ""
assert_eq "hook-upgrade-version" "$("$(bin_of "$h")" version)" "canary $manifest_v"
assert_eq "hook-upgrade-stamp"   "$(cat "$h/.canary/plugin-version")" "$manifest_v"

# a settings.json it will not touch (comments in it): the hook says so instead of
# staying silent about a status line that never appeared
h2=$(newhome); mkdir -p "$h2/.claude" "$h2/.local/bin" "$h2/.canary"
printf '// mine\n{}\n' > "$h2/.claude/settings.json"
cp "$(bin_of "$h")" "$(bin_of "$h2")"; echo "$manifest_v" > "$h2/.canary/plugin-version"
out=$(run_hook "$h2" "$bare_path")
assert_has "hook-untouched-says-so" "$out" "left untouched"
assert_eq  "hook-untouched-left"    "$(cat "$h2/.claude/settings.json")" "$(printf '// mine\n{}')"

# somebody else's binary on PATH (Homebrew, go install): wired, never reinstalled, no stamp
h=$(newhome); mkdir -p "$h/.claude" "$h/brew"; echo '{}' > "$h/.claude/settings.json"
printf '#!/bin/sh\n[ "$1" = version ] && echo canary 9.9.9\nexit 0\n' > "$h/brew/canary"; chmod +x "$h/brew/canary"
out=$(run_hook "$h" "$h/brew:$bare_path")
assert_eq "hook-foreign-silent" "$out" ""
[ -e "$(bin_of "$h")" ]        && { echo "FAIL [hook-foreign-installed]: reinstalled over somebody else's binary"; fails=$((fails+1)); }
[ -e "$h/.canary/plugin-version" ] && { echo "FAIL [hook-foreign-stamp]: stamped a binary it does not own"; fails=$((fails+1)); }

# render.Frames(Fresh) is the pattern the installer sings while it works
mixed=$(grep 'mixedFrames *=' "$HERE/internal/render/animate.go" | grep -o '"[^"]*"' | tr -d '"')
for f in $mixed; do
  case $installer in
    *"$f"*) ;;
    *) echo "FAIL [install-note-parity]: fresh animates '$f', install.sh does not"; fails=$((fails+1)) ;;
  esac
done

if [ "$fails" -eq 0 ]; then echo "ok — all install/uninstall checks passed"; else echo "$fails check(s) failed"; exit 1; fi
