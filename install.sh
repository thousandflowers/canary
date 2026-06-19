#!/usr/bin/env sh
# canary installer — wires the fatigue bird into Claude Code's statusLine.
# Idempotent: safe to run again. Curl-friendly.
#
#   sh install.sh                       # install from a cloned repo
#   curl -fsSL <raw>/install.sh | sh    # remote install (pulls from REPO_RAW)

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

# --- wire the bird into Claude Code's statusLine (non-destructive) ------------
# Claude Code allows one statusLine command, so canary is *appended* to any
# existing one (e.g. caveman's) — both render. A backup is saved first.
wire_statusline() {
  cfg="${CLAUDE_CONFIG_DIR:-$HOME/.claude}"
  settings="$cfg/settings.json"
  sl="$CANARY_HOME/canary-statusline.sh"
  add="bash \"$sl\""

  if ! command -v jq >/dev/null 2>&1; then
    echo "canary: jq not found — add this to $settings manually:"
    echo "  \"statusLine\": { \"type\": \"command\", \"command\": \"<existing>; $add\" }"
    return 0
  fi
  [ -d "$cfg" ] || { echo "canary: no Claude Code config at $cfg — skipping"; return 0; }
  [ -f "$settings" ] || echo '{}' > "$settings"

  # JSONC tolerance: if jq can't parse it (comments?), don't risk corrupting it
  if ! jq empty "$settings" >/dev/null 2>&1; then
    echo "canary: $settings isn't plain JSON (comments?) — append '; $add' to statusLine.command manually"
    return 0
  fi

  cur=$(jq -r '.statusLine.command // ""' "$settings")
  case "$cur" in
    *canary-statusline*) echo "canary: statusline already wired"; return 0 ;;
  esac
  if [ -n "$cur" ]; then
    newcmd="$cur; $add"        # keep whatever exists (caveman, starship…), append the bird
  else
    newcmd="$add"
  fi

  tmp=$(mktemp 2>/dev/null || echo "$settings.canary.tmp")
  if jq --arg c "$newcmd" '.statusLine = {type:"command", command:$c}' "$settings" > "$tmp"; then
    cp "$settings" "$settings.canary.bak"
    mv "$tmp" "$settings"
    echo "canary: statusline wired into $settings (backup: $settings.canary.bak)"
  else
    rm -f "$tmp"
    echo "canary: could not update $settings — add '; $add' to statusLine.command manually"
  fi
}

main() {
  mkdir -p "$CANARY_HOME"
  fetch canary-statusline.sh "$CANARY_HOME/canary-statusline.sh"
  chmod +x "$CANARY_HOME/canary-statusline.sh" 2>/dev/null || true
  wire_statusline

  printf '\n ▗███▖\n▐ O ▌>   canary wired into Claude Code\n\n'
  echo "restart Claude Code (or reload the window) to meet your bird."
}

main "$@"
