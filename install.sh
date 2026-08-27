#!/usr/bin/env sh
# canary installer — detects your shell, drops canary in ~/.canary, wires the rc.
# Idempotent: safe to run again. Curl-friendly.
#
#   sh install.sh            # install from a cloned repo
#   curl -fsSL <raw>/install.sh | sh   # remote install (pulls files from REPO_RAW)

set -eu

CANARY_HOME="$HOME/.canary"
REPO_RAW="https://raw.githubusercontent.com/thousandflowers/canary/main"
REPO_TARBALL="https://codeload.github.com/thousandflowers/canary/tar.gz/refs/heads/main"
# Marker for the login-shell chain line, so uninstall.sh can take back exactly
# what we added and nothing else.
CANARY_CHAIN_MARK='# canary — let login shells read .bashrc'

# where this script lives (empty when piped through curl)
SCRIPT_DIR=""
case "${0:-}" in
  # `|| SCRIPT_DIR=""` rather than `&& pwd || true`: the latter is SC2015 and
  # reads like if-then-else when it isn't. Failing to resolve is fine — we just
  # fall back to fetching over the network.
  */*) SCRIPT_DIR=$(cd "$(dirname "$0")" 2>/dev/null && pwd) || SCRIPT_DIR="" ;;
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

# --- the phrase corpus the statusline bird speaks from -----------------------
# Copied whole rather than fetched file by file: it is twenty-odd small files and
# a hand-maintained manifest would rot the first time somebody adds a category.
# Local copy first (brew, or a git clone), tarball only when piped through curl.
# Optional: with no corpus the bird simply never speaks, everything else works.
fetch_phrases() {
  mkdir -p "$CANARY_HOME/phrases"
  if [ -n "$SCRIPT_DIR" ] && [ -d "$SCRIPT_DIR/phrases" ]; then
    cp -R "$SCRIPT_DIR/phrases/." "$CANARY_HOME/phrases/"
    return $?
  fi
  command -v curl >/dev/null 2>&1 || return 1
  # an exact member path, not a glob: BSD tar and GNU tar disagree about
  # --wildcards, and this has to work on both.
  curl -fsSL "$REPO_TARBALL" \
    | tar -xzf - -C "$CANARY_HOME" --strip-components=1 canary-main/phrases 2>/dev/null
}

# --- detect shell + rc file --------------------------------------------------
detect_rc() {
  shell_name=$(basename "${SHELL:-/bin/sh}")
  case "$shell_name" in
    zsh)  echo "zsh|$HOME/.zshrc|canary.sh" ;;
    # Always ~/.bashrc, never .bash_profile. Interactive NON-login bash — what
    # a Linux terminal opens — reads only .bashrc, so wiring .bash_profile
    # there installs a bird that never loads. macOS terminals open LOGIN
    # shells, which read .bash_profile and never .bashrc unless it is sourced
    # from there, so main() also makes the login file chain to .bashrc.
    bash) echo "bash|$HOME/.bashrc|canary.sh" ;;
    fish) echo "fish|$HOME/.config/fish/config.fish|canary.fish" ;;
    *)    echo "sh|$HOME/.profile|canary.sh" ;;
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

# --- wire the bird into Claude Code's statusLine (best-effort, non-destructive)
# Appends `; bash canary-statusline.sh` to any existing statusLine command (e.g.
# caveman's) so both render — Claude Code allows only one statusLine command, and
# caveman prints [CAVEMAN] with no trailing newline, so the bird lands beside it.
wire_statusline() {
  cfg="${CLAUDE_CONFIG_DIR:-$HOME/.claude}"
  settings="$cfg/settings.json"
  sl="$CANARY_HOME/canary-statusline.sh"
  add="bash \"$sl\""

  if ! command -v jq >/dev/null 2>&1; then
    echo "canary: jq not found — skipping statusline. add manually:"
    echo "  \"statusLine\": { \"type\": \"command\", \"command\": \"<existing>; $add\" }"
    return 0
  fi
  [ -d "$cfg" ] || { echo "canary: no Claude Code config at $cfg — skipping statusline"; return 0; }
  [ -f "$settings" ] || echo '{}' > "$settings"

  # JSONC tolerance: if jq can't parse it (comments?), don't risk corrupting it
  if ! jq empty "$settings" >/dev/null 2>&1; then
    echo "canary: $settings isn't plain JSON (comments?) — add manually:"
    echo "  append '; $add' to your statusLine.command"
    return 0
  fi

  cur=$(jq -r '.statusLine.command // ""' "$settings")
  case "$cur" in
    *canary-statusline*) echo "canary: statusline already wired"; return 0 ;;
  esac
  if [ -n "$cur" ]; then
    newcmd="$cur; $add"        # keep caveman (or whatever exists), append the bird
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
  [ "$shell_name" = bash ] && ensure_bash_chain

  # Claude Code statusline (optional; needs jq + a Claude Code config dir)
  if fetch "canary-statusline.sh" "$CANARY_HOME/canary-statusline.sh" 2>/dev/null; then
    chmod +x "$CANARY_HOME/canary-statusline.sh" 2>/dev/null || true
  fi
  fetch_phrases || echo "canary: no phrase corpus installed — the bird will stay quiet"
  wire_statusline || true

  printf '\n ▗███▖\n▐ O ▌>   canary installed for %s\n\n' "$shell_name"
  echo "open a new shell (or: source $rc) to meet your bird."
}

main "$@"
