#!/usr/bin/env sh
# canary — pixel-art bird in your shell prompt that tracks cognitive fatigue.
# Zero deps. Pure POSIX-ish shell + UTF-8 block art. No ANSI color, no API calls.
# Source me from .zshrc / .bashrc:   source ~/.canary/canary.sh
#
# Knobs (env vars):
#   CANARY_DISABLED=1     turn the bird off
#   CANARY_RESET=1        reset session counters on next prompt
#   CANARY_SHOW_SCORE=1   print the numeric fatigue score next to the bird

# --- bail out if disabled ----------------------------------------------------
[ -n "${CANARY_DISABLED:-}" ] && return 0 2>/dev/null

# --- load guard (safe to re-source) ------------------------------------------
if [ -n "${_CANARY_LOADED:-}" ]; then
  return 0 2>/dev/null
fi
_CANARY_LOADED=1

# --- session state (set once per shell) --------------------------------------
: "${CANARY_START_TIME:=$(date +%s)}"
: "${CANARY_PROMPT_COUNT:=0}"
: "${CANARY_LENS:=}"          # space-separated lengths of last 20 commands
: "${CANARY_ACTIVE_SECONDS:=0}"               # accrued active (non-idle) seconds
: "${CANARY_LAST_ACTIVE:=$CANARY_START_TIME}" # epoch of the last recorded command
: "${CANARY_STATE_FILE:=$HOME/.canary/canary-state}"  # the Claude Code statusline reads this
[ -d "${CANARY_STATE_FILE%/*}" ] || mkdir -p "${CANARY_STATE_FILE%/*}" 2>/dev/null

# --- tunables ----------------------------------------------------------------
_CANARY_LEN_WINDOW=20                 # rolling window for avg prompt length
: "${CANARY_NIGHT_START:=22}"         # circadian penalty starts at/after this hour
: "${CANARY_NIGHT_END:=7}"            # ...and before this hour
: "${CANARY_NIGHT_MULT:=130}"         # penalty as percent: 100 = off, 130 = x1.3
: "${CANARY_IDLE_THRESHOLD:=300}"     # gaps longer than this (sec) count as a break, not work
: "${CANARY_MIN_SCORE:=0}"            # only draw the bird at/above this score (0 = always; 46 = tired+)
# score = minutes/3 + count/2 + avglen/10   (each term caps a band, total 0-100)
#   == (min/120*40) + (count/80*40) + (avglen/200*20)   from the spec
# NOTE: a playful *activity* proxy, not a real measure of cognitive load.

# --- record one executed command --------------------------------------------
_canary_record() {
  # zsh does not word-split unquoted expansions; opt in, locally, for the splits below
  [ -n "${ZSH_VERSION:-}" ] && setopt localoptions shwordsplit 2>/dev/null
  cmd=$1
  # ignore empty lines (bare Enter)
  [ -z "$cmd" ] && return 0

  # accrue active time, ignoring idle gaps (coffee breaks don't tire the bird)
  now=$(date +%s)
  gap=$(( now - CANARY_LAST_ACTIVE ))
  [ "$gap" -le "$CANARY_IDLE_THRESHOLD" ] && CANARY_ACTIVE_SECONDS=$(( CANARY_ACTIVE_SECONDS + gap ))
  CANARY_LAST_ACTIVE=$now

  CANARY_PROMPT_COUNT=$(( CANARY_PROMPT_COUNT + 1 ))
  len=${#cmd}
  CANARY_LENS="$CANARY_LENS $len"
  # keep only the last _CANARY_LEN_WINDOW samples
  # shellcheck disable=SC2086
  set -- $CANARY_LENS
  while [ "$#" -gt "$_CANARY_LEN_WINDOW" ]; do shift; done
  CANARY_LENS="$*"

  _canary_write_state
}

# --- rolling average of recorded lengths -------------------------------------
_canary_avg() {
  # zsh does not word-split unquoted expansions; opt in, locally
  [ -n "${ZSH_VERSION:-}" ] && setopt localoptions shwordsplit 2>/dev/null
  # shellcheck disable=SC2086
  set -- $CANARY_LENS
  n=$#
  if [ "$n" -eq 0 ]; then
    echo 0
    return 0
  fi
  sum=0
  for x in "$@"; do
    sum=$(( sum + x ))
  done
  echo $(( sum / n ))
}

# --- persist session state for the Claude Code statusline --------------------
_canary_write_state() {
  [ -n "${CANARY_STATE_FILE:-}" ] || return 0
  printf 'timestamp_start=%s\nprompt_count=%s\navg_prompt_len=%s\nactive_seconds=%s\n' \
    "$CANARY_START_TIME" "$CANARY_PROMPT_COUNT" "$(_canary_avg)" "$CANARY_ACTIVE_SECONDS" \
    > "$CANARY_STATE_FILE" 2>/dev/null
}

# --- map a 0-100 score to the bird art, set CANARY_BIRD, print it ------------
_canary_render() {
  score=$1
  force=${2:-}                 # non-empty -> always show the score line
  if   [ "$score" -le 20 ]; then name=fresh;  l1=' ▗███▖';   l2='▐ O ▌>'
  elif [ "$score" -le 45 ]; then name=chirpy; l1=' ▗███▖ ♪'; l2='▐ ^ ▌>'
  elif [ "$score" -le 70 ]; then name=tired;  l1=' ▗███▖';  l2='▐ - ▌>'
  elif [ "$score" -le 90 ]; then name=worn;   l1=' ▗▓▓▓▖';  l2='▐ ~ ▌>'
  else                           name=dead;   l1=' ▗░░░▖';  l2='░ x ▌v'
  fi

  CANARY_BIRD="$l1
$l2"

  if [ -n "${CANARY_SHOW_SCORE:-}" ] || [ -n "$force" ]; then
    printf '%s\n%s  [%s %s]\n' "$l1" "$l2" "$name" "$score"
  else
    printf '%s\n%s\n' "$l1" "$l2"
  fi

  if [ "$name" = dead ]; then
    printf '%s\n' '  tweet… you look fried. reset with  CANARY_RESET=1'
  fi
}

# --- compute the 0-100 fatigue score from current session state -------------
_canary_score() {
  min=$(( CANARY_ACTIVE_SECONDS / 60 ))      # active minutes (idle excluded)
  avglen=$(_canary_avg)
  s=$(( min / 3 + CANARY_PROMPT_COUNT / 2 + avglen / 10 ))

  # circadian penalty (configurable; set CANARY_NIGHT_MULT=100 to disable)
  hour=$(( 10#$(date +%H) ))
  if [ "$hour" -ge "$CANARY_NIGHT_START" ] || [ "$hour" -lt "$CANARY_NIGHT_END" ]; then
    s=$(( s * CANARY_NIGHT_MULT / 100 ))
  fi

  [ "$s" -gt 100 ] && s=100
  echo "$s"
}

# --- per-prompt: honor reset, recompute, draw -------------------------------
_canary_precmd() {
  [ -n "${CANARY_DISABLED:-}" ] && return 0

  if [ -n "${CANARY_RESET:-}" ]; then
    CANARY_START_TIME=$(date +%s)
    CANARY_PROMPT_COUNT=0
    CANARY_LENS=""
    CANARY_ACTIVE_SECONDS=0
    CANARY_LAST_ACTIVE=$CANARY_START_TIME
    unset CANARY_RESET
    _canary_write_state
  fi

  # bash: arm the preexec flag for the next typed command
  _CANARY_AT_PROMPT=1

  score=$(_canary_score)
  [ "$score" -lt "$CANARY_MIN_SCORE" ] && return 0   # stay quiet below the threshold
  _canary_render "$score"
}

# --- `canary` command: on-demand status / control ---------------------------
canary() {
  case "${1:-status}" in
    status|--status|"")
      _canary_render "$(_canary_score)" show ;;
    score|--score)
      _canary_score ;;
    reset|--reset)
      CANARY_START_TIME=$(date +%s); CANARY_PROMPT_COUNT=0; CANARY_LENS=""
      CANARY_ACTIVE_SECONDS=0; CANARY_LAST_ACTIVE=$CANARY_START_TIME
      _canary_write_state
      echo "canary: reset"; _canary_render "$(_canary_score)" show ;;
    off)
      CANARY_DISABLED=1; echo "canary: off (unset CANARY_DISABLED to re-enable)" ;;
    on)
      unset CANARY_DISABLED; echo "canary: on" ;;
    -h|--help|help)
      printf 'usage: canary [status|score|reset|on|off]\n' ;;
    *)
      printf 'canary: unknown command: %s\n' "$1" >&2; return 1 ;;
  esac
}

# --- hook registration, per shell -------------------------------------------
if [ -n "${ZSH_VERSION:-}" ]; then
  autoload -Uz add-zsh-hook 2>/dev/null
  _canary_preexec() { _canary_record "$1"; }
  add-zsh-hook preexec _canary_preexec
  add-zsh-hook precmd  _canary_precmd

elif [ -n "${BASH_VERSION:-}" ]; then
  # preexec emulation via DEBUG trap, gated by a once-per-prompt flag
  _canary_debug() {
    [ -n "${_CANARY_AT_PROMPT:-}" ] || return 0
    _CANARY_AT_PROMPT=""
    _canary_record "$BASH_COMMAND"
  }
  trap '_canary_debug' DEBUG

  # precmd via PROMPT_COMMAND (don't clobber an existing one)
  case "${PROMPT_COMMAND:-}" in
    *_canary_precmd*) : ;;
    "")  PROMPT_COMMAND="_canary_precmd" ;;
    *)   PROMPT_COMMAND="_canary_precmd; $PROMPT_COMMAND" ;;
  esac
fi
