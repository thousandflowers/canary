#!/usr/bin/env sh
# canary uninstaller — takes back the rc lines, the status line, ~/.canary and
# the binary install.sh put in ~/.local/bin.
# Idempotent: safe to run if canary isn't installed.
#
# Installed with Homebrew? `brew uninstall canary` removes the binary; run this
# first to unwire the rc and Claude Code.

set -eu

CANARY_HOME="$HOME/.canary"
CANARY_BIN_DIR="${CANARY_BIN_DIR:-$HOME/.local/bin}"
CANARY_BIN="$CANARY_BIN_DIR/canary"

# Lines canary owns in an rc file. Kept in one place so the "is there anything
# to do?" test and the filter below can never drift apart.
CANARY_MARK1='canary — fatigue bird'
# The hook line, in every shape install.sh writes it, plus the source line the
# pre-1.0 shell version used, so upgrading and then uninstalling is clean.
CANARY_MARK2='canary" init |\.canary/canary\.'
# install.sh adds a "source ~/.bashrc" line to the login rc so macOS login
# shells see the bird at all. Take back exactly that — anchored on our own
# marker comment and the exact line we wrote, never a .bashrc chain the user
# put there themselves.
CANARY_MARK3='canary — let login shells read \.bashrc'
# single-quoted on purpose: this is a regex matching a literal $HOME in the file
# shellcheck disable=SC2016
CANARY_MARK4='^\[ -f "\$HOME/\.bashrc" \] && \. "\$HOME/\.bashrc"$'

# The PATH lines are matched literally and in full, never by pattern: a PATH
# export is exactly the kind of line somebody else's installer also writes, and
# a regex here would take theirs down with ours.
CANARY_PATH_SH="export PATH=\"$CANARY_BIN_DIR:\$PATH\""
CANARY_PATH_FISH="fish_add_path $CANARY_BIN_DIR"

# --- follow a symlink chain to the real file ---------------------------------
# ~/.zshrc is very often a symlink into a dotfiles repo. Writing *through* the
# link is the only correct move: replacing the link with a regular file leaves
# the dotfiles copy still sourcing canary AND silently detaches the rc from the
# repo forever.
resolve_link() {
  p=$1
  d=0
  while [ -L "$p" ] && [ "$d" -lt 16 ]; do
    t=$(readlink "$p") || return 1
    case "$t" in
      /*) p=$t ;;
      *)  p="$(dirname "$p")/$t" ;;
    esac
    d=$(( d + 1 ))
  done
  printf '%s' "$p"
}

strip_rc() {
  rc=$1
  [ -e "$rc" ] || return 0
  rc=$(resolve_link "$rc") || return 0
  [ -f "$rc" ] && [ -w "$rc" ] || return 0

  # nothing of ours in here — don't rewrite the file at all
  grep -qE -e "$CANARY_MARK1" -e "$CANARY_MARK2" -e "$CANARY_MARK3" "$rc" 2>/dev/null || return 0

  tmp="$rc.canary.tmp.$$"
  # The bare `. ~/.bashrc` line is only ours when our marker comment sits in the
  # same file. Without that check we would delete a chain the user wrote long
  # before installing canary.
  if grep -qE -e "$CANARY_MARK3" "$rc" 2>/dev/null; then
    set -- -e "$CANARY_MARK1" -e "$CANARY_MARK2" -e "$CANARY_MARK3" -e "$CANARY_MARK4"
  else
    set -- -e "$CANARY_MARK1" -e "$CANARY_MARK2"
  fi

  # grep -v exits 1 when it selects no lines (an rc that was *only* canary):
  # a legitimate empty result. Exit >= 2 is a real read error — bail out and
  # leave the file untouched rather than truncate it to nothing.
  grep -vE "$@" "$rc" > "$tmp" 2>/dev/null || {
    st=$?
    if [ "$st" -gt 1 ]; then
      unlink "$tmp" 2>/dev/null || true
      echo "canary: could not read $rc — left untouched" >&2
      return 0
    fi
  }

  # Then the PATH lines, matched whole and literal.
  grep -vxF -e "$CANARY_PATH_SH" -e "$CANARY_PATH_FISH" "$tmp" > "$tmp.2" 2>/dev/null || {
    st=$?
    [ "$st" -gt 1 ] && cp "$tmp" "$tmp.2"
  }
  mv "$tmp.2" "$tmp" 2>/dev/null || true

  # A .bash_profile that now holds nothing is one install.sh created purely to
  # chain to .bashrc — it only ever does that when no login rc existed at all,
  # so removing it shadows nothing and leaves no litter behind.
  if [ ! -s "$tmp" ] && [ "$rc" = "$HOME/.bash_profile" ]; then
    unlink "$tmp" 2>/dev/null || true
    rm -f "$rc" && echo "canary: removed $rc (canary created it, nothing else in it)"
    return 0
  fi

  # write in place: keeps the inode, so permissions, ownership, hardlinks and
  # any symlink pointing here all survive. (mv would replace all of them.)
  if cat "$tmp" > "$rc" 2>/dev/null; then
    echo "canary: cleaned $rc"
  else
    echo "canary: could not write $rc — left untouched" >&2
  fi
  unlink "$tmp" 2>/dev/null || true
}

# --- remove the bird from Claude Code's statusLine, keeping anything else -----
# The binary knows how; jq is no longer involved. If canary is already gone from
# the system there is nothing here that can safely edit that JSON, so say so
# rather than guess with sed.
unwire_statusline() {
  for bin in "$CANARY_BIN" canary; do
    if command -v "$bin" >/dev/null 2>&1; then
      "$bin" settings remove || true
      return 0
    fi
  done
  echo "canary: binary already gone — remove canary from statusLine.command by hand if it is still there"
}

unwire_statusline

for rc in \
  "$HOME/.zshrc" \
  "$HOME/.bashrc" \
  "$HOME/.bash_profile" \
  "$HOME/.bash_login" \
  "$HOME/.profile" \
  "$HOME/.config/fish/config.fish"
do
  strip_rc "$rc"
done

if [ -d "$CANARY_HOME" ]; then
  rm -rf "$CANARY_HOME"
  echo "canary: removed $CANARY_HOME"
fi

# Only the binary install.sh placed. A Homebrew install lives elsewhere and is
# brew's to remove.
if [ -f "$CANARY_BIN" ]; then
  rm -f "$CANARY_BIN"
  echo "canary: removed $CANARY_BIN"
fi

printf '\n  x_x   canary uninstalled. open a new shell to be sure.\n\n'
