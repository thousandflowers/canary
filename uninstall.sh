#!/usr/bin/env sh
# canary uninstaller — removes the rc source line(s) and ~/.canary.
# Idempotent: safe to run if canary isn't installed.

set -eu

CANARY_HOME="$HOME/.canary"

strip_rc() {
  rc=$1
  [ -f "$rc" ] || return 0
  tmp=$(mktemp 2>/dev/null || echo "$rc.canary.tmp")
  # drop our marker comment and any line that sources canary
  grep -v -e 'canary — fatigue bird' -e '\.canary/canary\.' "$rc" > "$tmp" 2>/dev/null || true
  mv "$tmp" "$rc"
  echo "canary: cleaned $rc"
}

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

for rc in \
  "$HOME/.zshrc" \
  "$HOME/.bashrc" \
  "$HOME/.bash_profile" \
  "$HOME/.profile" \
  "$HOME/.config/fish/config.fish"
do
  strip_rc "$rc"
done

if [ -d "$CANARY_HOME" ]; then
  rm -rf "$CANARY_HOME"
  echo "canary: removed $CANARY_HOME"
fi

printf '\n  x_x   canary uninstalled. open a new shell to be sure.\n\n'
