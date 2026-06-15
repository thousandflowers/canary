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

# --- tunables (named constants, no magic numbers) ----------------------------
_CANARY_LEN_WINDOW=20         # rolling window for avg prompt length
_CANARY_NIGHT_START=22        # circadian penalty kicks in at/after this hour
_CANARY_NIGHT_END=7           # ...and before this hour
# score = minutes/3 + count/2 + avglen/10   (each term caps a band, total 0-100)
#   == (min/120*40) + (count/80*40) + (avglen/200*20)   from the spec

# --- record one executed command --------------------------------------------
_canary_record() {
  # zsh does not word-split unquoted expansions; opt in, locally, for the splits below
  [ -n "${ZSH_VERSION:-}" ] && setopt localoptions shwordsplit 2>/dev/null
  cmd=$1
  # ignore empty lines (bare Enter)
  [ -z "$cmd" ] && return 0
  CANARY_PROMPT_COUNT=$(( CANARY_PROMPT_COUNT + 1 ))
  len=${#cmd}
  CANARY_LENS="$CANARY_LENS $len"
  # keep only the last _CANARY_LEN_WINDOW samples
  # shellcheck disable=SC2086
  set -- $CANARY_LENS
  while [ "$#" -gt "$_CANARY_LEN_WINDOW" ]; do shift; done
  CANARY_LENS="$*"
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

# --- map a 0-100 score to the bird art, set CANARY_BIRD, print it ------------
_canary_render() {
  score=$1
  if   [ "$score" -le 20 ]; then name=fresh;  l1=' ▗███▖';  l2='▐ ◉ ▌>'
  elif [ "$score" -le 45 ]; then name=chirpy; l1=' ▗███▖~'; l2='▐ ^ ▌>'
  elif [ "$score" -le 70 ]; then name=tired;  l1=' ▗███▖';  l2='▐ - ▌>'
  elif [ "$score" -le 90 ]; then name=worn;   l1=' ▗▓▓▓▖';  l2='▐ ~ ▌>'
  else                           name=dead;   l1=' ▗░░░▖';  l2='░ x ▌v'
  fi

  CANARY_BIRD="$l1
$l2"

  if [ -n "${CANARY_SHOW_SCORE:-}" ]; then
    printf '%s\n%s  [%s %s]\n' "$l1" "$l2" "$name" "$score"
  else
    printf '%s\n%s\n' "$l1" "$l2"
  fi

  if [ "$name" = dead ]; then
    printf '%s\n' '  tweet… you look fried. reset with  CANARY_RESET=1'
  fi
}

# --- per-prompt: recompute fatigue and draw the bird -------------------------
_canary_precmd() {
  [ -n "${CANARY_DISABLED:-}" ] && return 0

  # honor reset request
  if [ -n "${CANARY_RESET:-}" ]; then
    CANARY_START_TIME=$(date +%s)
    CANARY_PROMPT_COUNT=0
    CANARY_LENS=""
    unset CANARY_RESET
  fi

  now=$(date +%s)
  min=$(( (now - CANARY_START_TIME) / 60 ))
  count=$CANARY_PROMPT_COUNT
  avglen=$(_canary_avg)

  score=$(( min / 3 + count / 2 + avglen / 10 ))

  # circadian penalty: late night / early morning is 1.3x more tiring
  hour=$(( 10#$(date +%H) ))
  if [ "$hour" -ge "$_CANARY_NIGHT_START" ] || [ "$hour" -lt "$_CANARY_NIGHT_END" ]; then
    score=$(( score * 13 / 10 ))
  fi

  [ "$score" -gt 100 ] && score=100

  _canary_render "$score"

  # bash: arm the preexec flag for the next typed command
  _CANARY_AT_PROMPT=1
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
