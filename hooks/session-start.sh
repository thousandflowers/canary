#!/bin/sh
# canary's SessionStart hook, for the Claude Code plugin. Puts the bird in the
# status line on the first session after the plugin is installed, keeps the
# binary it installed at the plugin's version, and is a silent no-op the rest
# of the time. The shell rc is never touched: that is the point of installing
# it this way.
#
# A hook's stdout may land in the model's context, so this says nothing unless
# something went wrong, and then one line naming the command to run by hand.
set -u

root=${CLAUDE_PLUGIN_ROOT:-$(dirname "$0")/..}
# Where install.sh puts the binary, whichever way it was run — the plugin, the
# one-liner, a clone. The plugin keeps that one at its own version. A binary
# from Homebrew or `go install` lives elsewhere, is theirs to upgrade, and is
# never touched.
own="$HOME/.local/bin/canary"
# The plugin version that last installed it. Not the binary's own word, which
# can be a release ahead of the plugin: the installer downloads latest.
stamp="$HOME/.canary/plugin-version"
want=$(sed -n 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$root/.claude-plugin/plugin.json" 2>/dev/null)

# The plugin root is a checkout of the repository, so the installer is right
# here. It fetches the release binary rather than building one: this runs
# inside Claude Code's start-up, and a `go build` behind a slow module proxy
# is a stall with no visible cause. A source build is `sh install.sh` in the
# plugin's checkout, by hand. The installer wires the status line itself.
install() {
  if ! CANARY_FETCH=1 sh "$root/install.sh" --claude-only >/dev/null 2>&1; then
    echo "canary: install failed; run: sh \"$root/install.sh\" --claude-only"
    return 1
  fi
  mkdir -p "$HOME/.canary" && printf '%s\n' "$want" > "$stamp"
}

if [ -x "$own" ] && [ "$(cat "$stamp" 2>/dev/null)" != "$want" ]; then
  install
  exit 0
fi

# Hooks inherit whatever PATH the app was launched with, and a Claude Code
# started from the Dock has never heard of Homebrew. Then the plugin installs
# its own copy rather than guessing at directories; a status line already
# wired to another binary is left as it is.
bird=$(command -v canary 2>/dev/null || true)
[ -n "$bird" ] || [ ! -x "$own" ] || bird=$own

if [ -z "$bird" ]; then
  install
  exit 0
fi
if out=$("$bird" settings install 2>&1); then
  # `settings install` exits 0 when it chose to leave a settings.json alone
  # (comments in it, not an object): a choice worth a line, not a silence.
  case "$out" in *"left untouched"*) printf '%s\n' "$out" ;; esac
  exit 0
fi
echo "canary: could not wire the status line; run: \"$bird\" settings install"
exit 0
