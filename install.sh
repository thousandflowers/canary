#!/usr/bin/env sh
# canary installer — puts the binary somewhere runnable and wires your rc.
# Idempotent: safe to run again. Curl-friendly.
#
#   sh install.sh                       # from a cloned repo (builds, if you have Go)
#   curl -fsSL <raw>/install.sh | sh    # remote install (downloads the release)
#   sh install.sh --claude-only         # only Claude Code's bird, rc untouched
#
# Homebrew is the cleaner path and does the first half of this for you:
#   brew install thousandflowers/tap/canary && canary settings install
#
# What changed at v1.0: canary is one Go binary. There is no canary.sh to copy
# into ~/.canary any more, no phrase corpus to install beside it (it is compiled
# in), and no jq — the binary edits Claude Code's settings.json itself.

set -eu

REPO="thousandflowers/canary"
CANARY_BIN_DIR="${CANARY_BIN_DIR:-$HOME/.local/bin}"
CANARY_BIN="$CANARY_BIN_DIR/canary"

# Markers, so uninstall.sh can take back exactly what we added and nothing else.
CANARY_RC_MARK='# canary — fatigue bird'
CANARY_CHAIN_MARK='# canary — let login shells read .bashrc'

# where this script lives (empty when piped through curl)
SCRIPT_DIR=""
case "${0:-}" in
  # `|| SCRIPT_DIR=""` rather than `&& pwd || true`: the latter is SC2015 and
  # reads like if-then-else when it isn't.
  */*) SCRIPT_DIR=$(cd "$(dirname "$0")" 2>/dev/null && pwd) || SCRIPT_DIR="" ;;
esac

# --- os_arch, in the spelling the release assets use -------------------------
platform() {
  os=$(uname -s)
  arch=$(uname -m)
  case "$os" in
    Darwin) os=darwin ;;
    Linux)  os=linux ;;
    *) echo "canary: no release build for $os — install Go and run: go install github.com/$REPO/cmd/canary@latest" >&2; return 1 ;;
  esac
  case "$arch" in
    arm64|aarch64) arch=arm64 ;;
    x86_64|amd64)  arch=amd64 ;;
    *) echo "canary: no release build for $arch — install Go and run: go install github.com/$REPO/cmd/canary@latest" >&2; return 1 ;;
  esac
  echo "${os}_${arch}"
}

# --- get a canary binary into $CANARY_BIN ------------------------------------
# Source first when we are in a clone with Go available: it is faster than the
# network and it installs exactly the tree you are looking at.
install_binary() {
  mkdir -p "$CANARY_BIN_DIR"

  if [ -n "$SCRIPT_DIR" ] && [ -f "$SCRIPT_DIR/go.mod" ] && command -v go >/dev/null 2>&1; then
    echo "canary: building from source"
    ( cd "$SCRIPT_DIR" && go build -o "$CANARY_BIN" ./cmd/canary )
    return 0
  fi
  if [ -n "$SCRIPT_DIR" ] && [ -x "$SCRIPT_DIR/canary" ] && [ ! -d "$SCRIPT_DIR/canary" ]; then
    cp "$SCRIPT_DIR/canary" "$CANARY_BIN"
    chmod +x "$CANARY_BIN"
    return 0
  fi

  command -v curl >/dev/null 2>&1 || {
    echo "canary: need curl to download a release (or Go, to build from source)" >&2
    return 1
  }
  plat=$(platform) || return 1
  url="https://github.com/$REPO/releases/latest/download/canary_${plat}.tar.gz"
  tmp=$(mktemp -d "${TMPDIR:-/tmp}/canary.XXXXXX")
  echo "canary: downloading $plat"
  if curl -fsSL "$url" | tar -xzf - -C "$tmp" 2>/dev/null && [ -f "$tmp/canary" ]; then
    cp "$tmp/canary" "$CANARY_BIN"
    chmod +x "$CANARY_BIN"
    rm -rf "$tmp"
    return 0
  fi
  rm -rf "$tmp"
  echo "canary: could not fetch $url" >&2
  return 1
}

# --- detect shell + rc file --------------------------------------------------
detect_rc() {
  shell_name=$(basename "${SHELL:-/bin/sh}")
  case "$shell_name" in
    zsh)  echo "zsh|$HOME/.zshrc" ;;
    # Always ~/.bashrc, never .bash_profile. Interactive NON-login bash — what
    # a Linux terminal opens — reads only .bashrc, so wiring .bash_profile
    # there installs a bird that never loads. macOS terminals open LOGIN
    # shells, which read .bash_profile and never .bashrc unless it is sourced
    # from there, so main() also makes the login file chain to .bashrc.
    bash) echo "bash|$HOME/.bashrc" ;;
    fish) echo "fish|$HOME/.config/fish/config.fish" ;;
    # No hook for anything else, but the rc still gets the PATH line so the
    # `canary` command works; the bird just will not perch by itself.
    *)    echo "sh|$HOME/.profile" ;;
  esac
}

# --- make login bash read ~/.bashrc too --------------------------------------
# For a login shell bash reads the FIRST of .bash_profile / .bash_login /
# .profile that exists, and never .bashrc. macOS Terminal opens login shells,
# so without this the bird is wired into a file that shell never reads.
# Idempotent, and it edits in place so a dotfiles symlink survives.
ensure_bash_chain() {
  # single-quoted on purpose: $HOME must land in the rc file literally, so the
  # line keeps working if the home directory ever moves
  # shellcheck disable=SC2016
  chain='[ -f "$HOME/.bashrc" ] && . "$HOME/.bashrc"'
  for f in "$HOME/.bash_profile" "$HOME/.bash_login" "$HOME/.profile"; do
    [ -f "$f" ] || continue
    # already chains (ours or the user's own) — leave it alone
    grep -q '\.bashrc' "$f" 2>/dev/null && return 0
    printf '\n%s\n%s\n' "$CANARY_CHAIN_MARK" "$chain" >> "$f"
    echo "canary: login shells now read ~/.bashrc ($f)"
    return 0
  done
  # No login rc exists at all, so creating the one bash prefers shadows nothing.
  printf '%s\n%s\n' "$CANARY_CHAIN_MARK" "$chain" > "$HOME/.bash_profile"
  echo "canary: created $HOME/.bash_profile so login shells read ~/.bashrc"
}

# --- idempotent rc line ------------------------------------------------------
ensure_line() {
  rc=$1
  line=$2
  # Named, because the PATH line and the hook both come through here and two
  # identical "added the hook" lines read like the installer did it twice.
  what=${3:-hook}
  mkdir -p "$(dirname "$rc")" 2>/dev/null || true
  touch "$rc" 2>/dev/null || true
  if grep -qF "$line" "$rc" 2>/dev/null; then
    echo "canary: rc already has the $what ($rc)"
    return 0
  fi
  # A read-only rc used to print "added the hook" and change nothing: the
  # installer signed off, the bird never appeared, and there was no way to tell
  # why. An install that could not do the thing has to say so and fail.
  # -w first: a failed redirection makes the shell itself print "Permission
  # denied" on the way past, which lands in the middle of the animation.
  if [ -w "$rc" ] && printf '\n%s\n%s\n' "$CANARY_RC_MARK" "$line" >> "$rc" 2>/dev/null; then
    echo "canary: added the $what to $rc"
    return 0
  fi
  echo "canary: cannot write $rc — the $what was not added." >&2
  echo "canary: make it writable, or add this line yourself:" >&2
  echo "  $line" >&2
  return 1
}

# --- $CANARY_BIN_DIR on PATH, only when it is not already --------------------
# Homebrew installs somewhere already on PATH, so this whole step is for the
# curl path. The hook itself calls the binary by absolute path and does not
# need it; this is so you can type `canary`.
ensure_path() {
  shell_name=$1
  rc=$2
  case ":${PATH}:" in
    *":$CANARY_BIN_DIR:"*) return 0 ;;
  esac
  if [ "$shell_name" = fish ]; then
    ensure_line "$rc" "fish_add_path $CANARY_BIN_DIR" "PATH line" || return 1
  else
    ensure_line "$rc" "export PATH=\"$CANARY_BIN_DIR:\$PATH\"" "PATH line" || return 1
  fi
}


# --- the bird, drawn from here ----------------------------------------------
# The installer cannot ask the binary to draw: on the curl path there is no
# binary yet when the first frame goes up. Same fresh art as internal/render,
# and test_install_uninstall.sh fails if the two ever drift.
BIRD_TOP='▗███▖'
BIRD_BODY='▐ O ▌>'

# The note patterns are render.Frames(Fresh): rises, hesitates, falls. Every
# frame is the same width on purpose — a shorter one would shuffle the label
# sideways on every tick.
#
# In the status row notes move only during a real break (VOICE.md section 8),
# because nothing pretty there should be earnable by grinding. An install is
# not the status row: it happens once, it is already an interruption, and the
# bird has something to be pleased about.
NOTES='♪·· ·♪· ·♫· ·♪· ♪·· ···'
if [ -n "${CANARY_ASCII:-}" ]; then
  NOTES='o.. .o. .O. .o. o.. ...'
fi

# Animate only when someone is watching. A pipe, a CI log, NO_COLOR or a dumb
# terminal get the same words with no cursor games — the bird singing is a
# courtesy, never the way information is delivered.
ANIM=0
if [ -t 1 ] && [ -z "${NO_COLOR:-}" ] && [ -z "${CANARY_NO_ANIM:-}" ] && [ "${TERM:-dumb}" != dumb ]; then
  ANIM=1
fi

# Fractional sleep is not POSIX. Busybox sleep takes whole seconds only, and a
# one-second frame is not an animation, it is a slideshow — so the frames just
# run flat out there instead.
NAP=0
if sleep 0.05 2>/dev/null; then NAP=1; fi
nap() { if [ "$NAP" = 1 ]; then sleep "$1"; fi; }

cursor_hide() { if [ "$ANIM" = 1 ]; then printf '\033[?25l'; fi; }
cursor_show() { if [ "$ANIM" = 1 ]; then printf '\033[?25h'; fi; }

# Two rows, the note and the label in the slot beside the beak — the same slot
# the bird speaks from once it is installed.
frame() {
  printf ' %s\n%s  %s %s\033[K\n' "$BIRD_TOP" "$BIRD_BODY" "$1" "$2"
}
rewind() { printf '\033[2A'; }

# The work runs in the background and the bird sings until it is done. Step
# output goes to a log and is shown underneath afterwards: a build scrolling
# through the animation is neither a build log nor an animation.
stage() {
  label=$1
  shift
  if [ "$ANIM" != 1 ]; then
    printf 'canary: %s\n' "$label"
    "$@"
    return $?
  fi
  ( "$@" >>"$STEP_LOG" 2>&1 ) &
  pid=$!
  notes=$NOTES
  i=0
  # A step can finish before it has been seen. Six frames is one whole phrase
  # of the pattern, which is the least that reads as singing.
  while kill -0 "$pid" 2>/dev/null || [ "$i" -lt 6 ]; do
    frame "${notes%% *}" "$label"
    # rotate: first frame to the back, no arrays and no eval
    notes="${notes#* } ${notes%% *}"
    rewind
    i=$((i + 1))
    nap 0.11
  done
  st=0
  wait "$pid" || st=$?
  frame "${NOTES%% *}" "$label"
  rewind
  return "$st"
}

# --- what this machine actually is ------------------------------------------
tilde() { case $1 in "$HOME"/*) printf '~%s' "${1#"$HOME"}" ;; *) printf '%s' "$1" ;; esac; }

wire_shell() {
  ensure_path "$shell_name" "$rc" || return 1
  if [ -n "$hook" ]; then
    ensure_line "$rc" "$hook" || return 1
    if [ "$shell_name" = bash ]; then ensure_bash_chain; fi
  else
    echo "canary: no prompt hook for $shell_name — \`canary\` still works by hand"
  fi
}

# Claude Code's status line. No jq: the binary edits the JSON itself, and a
# machine without Claude Code is not a failed install.
install_statusline() { "$CANARY_BIN" settings install || true; }

usage() {
  cat <<'USAGE'
canary installer

  sh install.sh                 install the binary, wire the shell prompt and
                                Claude Code's status line
  sh install.sh --claude-only   install the binary and Claude Code's status line
                                only, and leave the shell rc alone
  sh install.sh --help          this

  CANARY_CLAUDE_ONLY=1          same as --claude-only, for `curl ... | sh`
  CANARY_BIN_DIR=DIR            where to put the binary (default ~/.local/bin)
USAGE
}

main() {
  # Two birds, and some people want only the one in Claude Code. The env var is
  # for the curl path, where passing a flag means `sh -s --`.
  claude_only="${CANARY_CLAUDE_ONLY:-0}"
  for arg in "$@"; do
    case "$arg" in
      --claude-only) claude_only=1 ;;
      -h|--help) usage; return 0 ;;
      *) echo "canary: unknown option: $arg" >&2; usage >&2; return 2 ;;
    esac
  done

  info=$(detect_rc)
  shell_name=${info%%|*}
  rc=${info#*|}

  case "$shell_name" in
    fish) hook="\"$CANARY_BIN\" init fish | source" ;;
    zsh|bash) hook="eval \"\$(\"$CANARY_BIN\" init $shell_name)\"" ;;
    *) hook="" ;;
  esac

  # What this machine actually is, before anything is touched. The curl path
  # runs sight-unseen; naming what was detected is the least it can do.
  if [ "$ANIM" = 1 ]; then
    plat=$(platform 2>/dev/null) || plat="source"
    printf '\ncanary — a retired safety instrument\n\n'
    printf '  platform  %s\n' "$plat"
    if [ "$claude_only" = 1 ]; then
      printf '  shell     %s (untouched)\n' "$shell_name"
    else
      printf '  shell     %s → %s\n' "$shell_name" "$(tilde "$rc")"
    fi
    printf '  binary    %s\n\n' "$(tilde "$CANARY_BIN")"
  fi

  STEP_LOG=$(mktemp "${TMPDIR:-/tmp}/canary-install.XXXXXX")
  # An interrupted install must not leave the terminal without a cursor.
  trap 'cursor_show; rm -f "$STEP_LOG"' EXIT
  trap 'cursor_show; rm -f "$STEP_LOG"; exit 130' INT TERM
  cursor_hide

  if [ -n "$SCRIPT_DIR" ] && [ -f "$SCRIPT_DIR/go.mod" ] && command -v go >/dev/null 2>&1; then
    fetch_label="building from source"
  else
    fetch_label="fetching the binary"
  fi
  stage "$fetch_label" install_binary || { cursor_show; cat "$STEP_LOG" >&2; return 1; }
  if [ "$claude_only" != 1 ]; then
    stage "wiring $shell_name" wire_shell || { cursor_show; cat "$STEP_LOG" >&2; return 1; }
  fi
  stage "claude code's status line" install_statusline

  cursor_show
  if [ "$ANIM" = 1 ]; then
    # Wipe the bird that sang. The one that signs off below is the same bird —
    # leaving both on screen makes an install look like it installed two.
    # The cursor is on the first of its two rows, and stays there: the step log
    # takes the space the animation was using.
    printf '\033[2K\033[1B\033[2K\033[1A\r'
    # Everything the steps said, where the bird was instead of under it.
    sed 's/^/  /' "$STEP_LOG"
  fi

  # The last word is the bird's own: the real binary it just installed, drawing
  # itself with a phrase from the corpus compiled into it. Fresh, because that
  # is the band a bird is in when the air is fine and nothing has gone wrong
  # yet. It doubles as a smoke test — a binary that cannot run cannot sign off.
  printf '\n'
  if ! env COLUMNS="${COLUMNS:-80}" "$CANARY_BIN" preview --state fresh 2>/dev/null; then
    printf ' ▗███▖\n▐ O ▌>   canary installed for %s\n' "$shell_name"
  fi
  if [ "$claude_only" = 1 ]; then
    printf '\nthe bird lives in Claude Code now; %s was not touched.\n' "$(tilde "$rc")"
    printf 'want it above your shell prompt too? add: %s\n' "$hook"
  else
    printf '\nopen a new shell (or: . %s) to meet it.\n' "$(tilde "$rc")"
  fi
}

main "$@"
