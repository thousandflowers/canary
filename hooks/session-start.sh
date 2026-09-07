#!/bin/sh
# canary's SessionStart hook, for the Claude Code plugin. Puts the bird in the
# status line on the first session after the plugin is installed; every later
# session is a no-op, `canary settings install` refuses to wire itself twice.
# The shell rc is never touched: that is the point of installing it this way.
#
# A hook's stdout lands in the model's context, so this says nothing unless
# something went wrong, and then one line naming the command to run by hand.
set -u

bird=$(command -v canary 2>/dev/null || true)
# Hooks inherit whatever PATH the app was launched with; a Claude Code started
# from the Dock has never heard of Homebrew or ~/.local/bin.
for candidate in "$HOME/.local/bin/canary" /opt/homebrew/bin/canary; do
  [ -n "$bird" ] && break
  [ -x "$candidate" ] && bird=$candidate
done

if [ -n "$bird" ]; then
  "$bird" settings install >/dev/null 2>&1 && exit 0
  echo "canary: could not wire the status line; run: \"$bird\" settings install"
  exit 0
fi

# No binary anywhere. The plugin root is a checkout of the repository, so the
# installer is right here: it builds from source when Go is around, fetches the
# release binary otherwise, then wires the status line itself.
root=${CLAUDE_PLUGIN_ROOT:-$(dirname "$0")/..}
sh "$root/install.sh" --claude-only >/dev/null 2>&1 && exit 0
echo "canary: install failed; run: sh \"$root/install.sh\" --claude-only"
exit 0
