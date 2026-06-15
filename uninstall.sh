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
