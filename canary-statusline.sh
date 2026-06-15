#!/usr/bin/env bash
# canary-statusline.sh — renders the fatigue bird for Claude Code's statusLine.
#
# Reads ~/.canary/canary-state (written by canary.sh's preexec hook) and prints
# two lines, designed to sit next to caveman's [CAVEMAN] badge (which emits no
# trailing newline, so our line 1 continues on the same row):
#
#   [CAVEMAN] ▗███▖ fresh · 12m · 8p
#             ▐ O ▌>
#
# No ANSI color. The scoring mirrors canary.sh — this runs as a separate process
# (Claude Code invokes it), so it can't share the interactive shell's variables.

STATE="${CANARY_STATE_FILE:-$HOME/.canary/canary-state}"

# refuse a symlinked state file (a local attacker could repoint it); no-op if absent
[ -L "$STATE" ] && exit 0
[ -f "$STATE" ] || exit 0

# parse only the keys we expect; keep digits only (blocks terminal-escape injection)
ts_start=0; prompt_count=0; avg_len=0; active=-1
while IFS='=' read -r k v; do
  v=$(printf '%s' "$v" | tr -cd '0-9')
  case "$k" in
    timestamp_start) ts_start=${v:-0} ;;
    prompt_count)    prompt_count=${v:-0} ;;
    avg_prompt_len)  avg_len=${v:-0} ;;
    active_seconds)  active=${v:--1} ;;
  esac
done < "$STATE"

# minutes: prefer idle-aware active_seconds; fall back to wall clock from start
if [ "$active" -ge 0 ]; then
  min=$(( active / 60 ))
else
  now=$(date +%s)
  min=$(( (now - ts_start) / 60 ))
  [ "$min" -lt 0 ] && min=0
fi

score=$(( min / 3 + prompt_count / 2 + avg_len / 10 ))

# circadian penalty (defaults; honors CANARY_NIGHT_* if exported into CC's env)
ns=${CANARY_NIGHT_START:-22}; ne=${CANARY_NIGHT_END:-7}; nm=${CANARY_NIGHT_MULT:-130}
hour=$(( 10#$(date +%H) ))
if [ "$hour" -ge "$ns" ] || [ "$hour" -lt "$ne" ]; then
  score=$(( score * nm / 100 ))
fi
[ "$score" -gt 100 ] && score=100

# optional quiet threshold — show nothing below it
[ "$score" -lt "${CANARY_MIN_SCORE:-0}" ] && exit 0

if   [ "$score" -le 20 ]; then state=fresh;  top='▗███▖';  eye='O'; beak='>'
elif [ "$score" -le 45 ]; then state=chirpy; top='▗███▖♪'; eye='^'; beak='>'
elif [ "$score" -le 70 ]; then state=tired;  top='▗███▖';  eye='-'; beak='>'
elif [ "$score" -le 90 ]; then state=worn;   top='▗▓▓▓▖';  eye='~'; beak='>'
else                           state=dead;   top='▗░░░▖';  eye='x'; beak='v'
fi

# line 1 (a leading space separates it from [CAVEMAN]); line 2 indented so the
# bird's lower half sits under its upper half when the default badge is shown.
printf ' %s %s · %dm · %dp\n          ▐ %s ▌%s' \
  "$top" "$state" "$min" "$prompt_count" "$eye" "$beak"
