#!/usr/bin/env sh
# canary uninstaller — removes the bird from Claude Code's statusLine and
# deletes ~/.canary. Idempotent: safe to run if canary isn't installed.

set -eu

CANARY_HOME="$HOME/.canary"

# --- remove the bird from Claude Code's statusLine, keeping anything else -----
unwire_statusline() {
  cfg="${CLAUDE_CONFIG_DIR:-$HOME/.claude}"
  settings="$cfg/settings.json"
  command -v jq >/dev/null 2>&1 || return 0
  [ -f "$settings" ] || return 0
  jq empty "$settings" >/dev/null 2>&1 || return 0
  cur=$(jq -r '.statusLine.command // ""' "$settings")
  case "$cur" in
    *canary-statusline*) ;;
    *) return 0 ;;
  esac
  # drop our segment; tidy up stray leading/trailing separators
  new=$(printf '%s' "$cur" | sed -E 's/;?[[:space:]]*bash "[^"]*canary-statusline\.sh"//g')
  new=$(printf '%s' "$new" | sed -E 's/^[[:space:]]*;[[:space:]]*//; s/[[:space:]]*;[[:space:]]*$//; s/^[[:space:]]*//; s/[[:space:]]*$//')
  tmp=$(mktemp 2>/dev/null || echo "$settings.canary.tmp")
  if [ -n "$new" ]; then
    jq --arg c "$new" '.statusLine.command = $c' "$settings" > "$tmp" && mv "$tmp" "$settings"
  else
    jq 'del(.statusLine)' "$settings" > "$tmp" && mv "$tmp" "$settings"
  fi
  echo "canary: statusline unwired from $settings"
}

unwire_statusline

if [ -d "$CANARY_HOME" ]; then
  rm -rf "$CANARY_HOME"
  echo "canary: removed $CANARY_HOME"
fi

printf '\n  x_x   canary uninstalled. restart Claude Code to be sure.\n\n'
