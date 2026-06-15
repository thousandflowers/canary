#!/usr/bin/env sh
# canary installer — detects your shell, drops canary in ~/.canary, wires the rc.
# Idempotent: safe to run again. Curl-friendly.
#
#   sh install.sh            # install from a cloned repo
#   curl -fsSL <raw>/install.sh | sh   # remote install (pulls files from REPO_RAW)

set -eu

CANARY_HOME="$HOME/.canary"
REPO_RAW="https://raw.githubusercontent.com/thousandflowers/canary/main"

# where this script lives (empty when piped through curl)
SCRIPT_DIR=""
case "${0:-}" in
  */*) SCRIPT_DIR=$(cd "$(dirname "$0")" 2>/dev/null && pwd || true) ;;
esac

# --- fetch a file: prefer local sibling, else curl from the repo -------------
fetch() {
  name=$1
  dest=$2
  if [ -n "$SCRIPT_DIR" ] && [ -f "$SCRIPT_DIR/$name" ]; then
    cp "$SCRIPT_DIR/$name" "$dest"
  elif command -v curl >/dev/null 2>&1; then
    curl -fsSL "$REPO_RAW/$name" -o "$dest"
  else
    echo "canary: cannot find $name locally and curl is missing." >&2
    return 1
  fi
}

# --- detect shell + rc file --------------------------------------------------
detect_rc() {
  shell_name=$(basename "${SHELL:-/bin/sh}")
  case "$shell_name" in
    zsh)  echo "zsh|$HOME/.zshrc|canary.sh" ;;
    bash) if [ -f "$HOME/.bashrc" ]; then
            echo "bash|$HOME/.bashrc|canary.sh"
          else
            echo "bash|$HOME/.bash_profile|canary.sh"
          fi ;;
    fish) echo "fish|$HOME/.config/fish/config.fish|canary.fish" ;;
    *)    echo "sh|$HOME/.profile|canary.sh" ;;
  esac
}

# --- idempotent source line --------------------------------------------------
ensure_line() {
  rc=$1
  line=$2
  mkdir -p "$(dirname "$rc")"
  touch "$rc"
  if grep -qF "$line" "$rc" 2>/dev/null; then
    echo "canary: rc already wired ($rc)"
  else
    printf '\n# canary — fatigue bird\n%s\n' "$line" >> "$rc"
    echo "canary: added source line to $rc"
  fi
}

main() {
  info=$(detect_rc)
  shell_name=${info%%|*}
  rest=${info#*|}
  rc=${rest%%|*}
  asset=${rest##*|}

  mkdir -p "$CANARY_HOME"
  fetch "$asset" "$CANARY_HOME/$asset"

  if [ "$shell_name" = fish ]; then
    line="test -f $CANARY_HOME/canary.fish; and source $CANARY_HOME/canary.fish"
  else
    line="[ -f \"$CANARY_HOME/$asset\" ] && . \"$CANARY_HOME/$asset\""
  fi
  ensure_line "$rc" "$line"

  printf '\n ▗███▖\n▐ ◉ ▌>   canary installed for %s\n\n' "$shell_name"
  echo "open a new shell (or: source $rc) to meet your bird."
}

main "$@"
