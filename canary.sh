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
: "${CANARY_LEN_SUM:=0}"                      # running total of command lengths
: "${CANARY_ACTIVE_SECONDS:=0}"               # accrued active (non-idle) seconds
: "${CANARY_LAST_ACTIVE:=$CANARY_START_TIME}" # epoch of the last recorded command
: "${CANARY_STATE_FILE:=$HOME/.canary/canary-state}"  # the Claude Code statusline reads this
[ -d "${CANARY_STATE_FILE%/*}" ] || mkdir -p "${CANARY_STATE_FILE%/*}" 2>/dev/null

# --- tunables ----------------------------------------------------------------
: "${CANARY_NIGHT_MULT:=150}"         # multiplier at the deepest circadian trough
                                      # (150 = x1.5 at 02:00-04:00); 100 = off
: "${CANARY_IDLE_THRESHOLD:=300}"     # gaps longer than this (sec) count as a break, not work
: "${CANARY_MIN_SCORE:=0}"            # only draw the bird at/above this score
                                      # (0 = always; 71 = worn+, the KSS>=7 line)
# score = time_points(active minutes) + count/2 + avglen/10, then the
# time-of-day multiplier, capped at 100. Bands map onto the five labelled
# anchors of the Karolinska Sleepiness Scale: fresh=KSS1, chirpy=KSS3,
# tired=KSS5 ("neither alert nor sleepy"), worn=KSS7 ("sleepy"), dead=KSS9.
# KSS>=7 is the level at which fatigue-risk protocols call for a break, which
# is why `worn`, not `tired`, is the band worth acting on.
# NOTE: still an *activity* proxy. The shape is evidence-based; the inputs are
# keystrokes, not a psychomotor vigilance task.

# --- record one executed command --------------------------------------------
_canary_record() {
  _cy_cmd=$1
  # ignore empty lines (bare Enter)
  [ -z "$_cy_cmd" ] && return 0

  # accrue active time, ignoring idle gaps (coffee breaks don't tire the bird)
  _cy_now=$(date +%s)
  _cy_gap=$(( _cy_now - CANARY_LAST_ACTIVE ))
  [ "$_cy_gap" -le "$CANARY_IDLE_THRESHOLD" ] && CANARY_ACTIVE_SECONDS=$(( CANARY_ACTIVE_SECONDS + _cy_gap ))
  CANARY_LAST_ACTIVE=$_cy_now

  CANARY_PROMPT_COUNT=$(( CANARY_PROMPT_COUNT + 1 ))
  CANARY_LEN_SUM=$(( CANARY_LEN_SUM + ${#_cy_cmd} ))

  _canary_write_state
}

# --- mean command length, over the whole session -----------------------------
# A running sum over the same commands PROMPT_COUNT counts. This used to keep a
# list of the last 20 lengths and shift it with `set --`, which needed `setopt
# shwordsplit` under zsh in two places — a lot of shell arcana for a term worth
# a few points out of 100, and inconsistent with `min` and `count`, which have
# always been cumulative.
_canary_avg() {
  [ "$CANARY_PROMPT_COUNT" -gt 0 ] || { echo 0; return 0; }
  echo $(( CANARY_LEN_SUM / CANARY_PROMPT_COUNT ))
}

# --- persist session state for the Claude Code statusline --------------------
_canary_write_state() {
  [ -n "${CANARY_STATE_FILE:-}" ] || return 0
  printf 'prompt_count=%s\navg_prompt_len=%s\nactive_seconds=%s\n' \
    "$CANARY_PROMPT_COUNT" "$(_canary_avg)" "$CANARY_ACTIVE_SECONDS" \
    > "$CANARY_STATE_FILE" 2>/dev/null
}

# --- map a 0-100 score to the bird art and print it --------------------------
_canary_render() {
  _cy_score=$1
  _cy_force=${2:-}             # non-empty -> always show the score line
  if   [ "$_cy_score" -le 20 ]; then _cy_name=fresh;  _cy_l1=' ▗███▖';   _cy_l2='▐ O ▌>'
  elif [ "$_cy_score" -le 45 ]; then _cy_name=chirpy; _cy_l1=' ▗███▖ ♪'; _cy_l2='▐ ^ ▌>'
  elif [ "$_cy_score" -le 70 ]; then _cy_name=tired;  _cy_l1=' ▗███▖';  _cy_l2='▐ - ▌>'
  elif [ "$_cy_score" -le 90 ]; then _cy_name=worn;   _cy_l1=' ▗▓▓▓▖';  _cy_l2='▐ ~ ▌>'
  else                               _cy_name=dead;   _cy_l1=' ▗░░░▖';  _cy_l2='░ x ▌v'
  fi

  if [ -n "${CANARY_SHOW_SCORE:-}" ] || [ -n "$_cy_force" ]; then
    printf '%s\n%s  [%s %s]\n' "$_cy_l1" "$_cy_l2" "$_cy_name" "$_cy_score"
  else
    printf '%s\n%s\n' "$_cy_l1" "$_cy_l2"
  fi

  if [ "$_cy_name" = dead ]; then
    printf '%s\n' '  tweet… you look fried. reset with  CANARY_RESET=1'
  fi
}

# --- active minutes -> points ------------------------------------------------
# Concave, not linear. The vigilance-decrement literature is consistent that the
# curve is front-loaded: about half the decrement lands in the first ~15 min,
# reaction times climb reliably past ~30 min, costs steepen after ~60 min, and
# then it flattens rather than continuing straight up.
#   15m->7  30m->14  1h->26  2h->43  3h->55  5h->72  8h->86  12h->97
# The old min/3 was a straight line: it under-read the first hour (60 min of
# solid work scored 20, still "chirpy") and pinned everything past 5h at 100,
# which turned the dead bird into wallpaper.
_canary_time_points() {
  echo $(( $1 * 130 / ($1 + 240) ))
}

# --- time-of-day: percentage points added, at full amplitude -----------------
# Shape from the circadian literature: the nadir runs 02:00-06:00 and is
# deepest 02:00-04:00; attention bottoms out 04:00-07:00; there is a real
# post-lunch dip 13:00-16:00 that is circadian, not dietary; and 17:00-21:00
# covers the evening "wake maintenance zone", where alertness is genuinely high.
# The old rule — a flat x1.3 from 22:00 to 07:00 — penalised you hardest during
# the part of the evening you are sharpest, and treated 23:00 like 03:00.
_canary_circadian_excess() {
  case $1 in
    2|3|4)          echo 50 ;;  # deepest trough
    5|6)            echo 40 ;;
    0|1)            echo 25 ;;
    7|13|14|15|23)  echo 15 ;;  # tail of the nadir; the post-lunch dip
    16|22)          echo 5 ;;
    *)              echo 0 ;;   # 08-12 morning peak, 17-21 wake maintenance
  esac
}

# --- compute the 0-100 fatigue score from current session state -------------
_canary_score() {
  _cy_min=$(( CANARY_ACTIVE_SECONDS / 60 ))  # active minutes (idle excluded)
  _cy_avglen=$(_canary_avg)
  _cy_s=$(( $(_canary_time_points "$_cy_min") + CANARY_PROMPT_COUNT / 2 + _cy_avglen / 10 ))

  # 10# forces base 10: `08`/`09` are invalid octal and would abort the shell.
  # shellcheck disable=SC3052  # bash/zsh/ksh only; dash never reaches a prompt hook
  _cy_hour=$(( 10#$(date +%H) ))
  # CANARY_NIGHT_MULT is the multiplier at the bottom of the trough; it scales
  # the whole curve, so 100 switches the time-of-day adjustment off entirely.
  _cy_excess=$(( $(_canary_circadian_excess "$_cy_hour") * (CANARY_NIGHT_MULT - 100) / 50 ))
  [ "$_cy_excess" -gt 0 ] && _cy_s=$(( _cy_s * (100 + _cy_excess) / 100 ))

  [ "$_cy_s" -gt 100 ] && _cy_s=100
  echo "$_cy_s"
}

# --- per-prompt: honor reset, recompute, draw -------------------------------
_canary_precmd() {
  [ -n "${CANARY_DISABLED:-}" ] && return 0

  if [ -n "${CANARY_RESET:-}" ]; then
    CANARY_START_TIME=$(date +%s)
    CANARY_PROMPT_COUNT=0
    CANARY_LEN_SUM=0
    CANARY_ACTIVE_SECONDS=0
    CANARY_LAST_ACTIVE=$CANARY_START_TIME
    unset CANARY_RESET
    _canary_write_state
  fi

  # bash: arm the preexec flag for the next typed command
  _CANARY_AT_PROMPT=1

  _cy_pscore=$(_canary_score)
  [ "$_cy_pscore" -lt "$CANARY_MIN_SCORE" ] && return 0  # stay quiet below the threshold
  _canary_render "$_cy_pscore"
}

# --- `canary` command: on-demand status / control ---------------------------
canary() {
  case "${1:-status}" in
    status)
      _canary_render "$(_canary_score)" show ;;
    score)
      _canary_score ;;
    reset)
      CANARY_START_TIME=$(date +%s); CANARY_PROMPT_COUNT=0; CANARY_LEN_SUM=0
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
  # preexec emulation via DEBUG trap, gated by a once-per-prompt flag.
  # SC3028/SC3047: BASH_COMMAND and trap DEBUG are bash-only — this whole branch
  # is guarded by $BASH_VERSION, so a POSIX sh never evaluates it.
  # shellcheck disable=SC3028
  _canary_debug() {
    [ -n "${_CANARY_AT_PROMPT:-}" ] || return 0
    _CANARY_AT_PROMPT=""
    _canary_record "$BASH_COMMAND"
  }
  # shellcheck disable=SC3047
  trap '_canary_debug' DEBUG

  # precmd via PROMPT_COMMAND (don't clobber an existing one)
  case "${PROMPT_COMMAND:-}" in
    *_canary_precmd*) : ;;
    "")  PROMPT_COMMAND="_canary_precmd" ;;
    *)   PROMPT_COMMAND="_canary_precmd; $PROMPT_COMMAND" ;;
  esac
fi
