#!/usr/bin/env bash
# canary-statusline.sh — the fatigue bird for Claude Code's statusLine.
#
# DUAL-MODE (zero runtime deps — pure shell + grep/awk, no jq, no ANSI color):
#
#   • Claude Code mode  — Claude Code pipes its session JSON on stdin every
#     refresh. We read `cost.total_duration_ms` for session minutes and walk the
#     `transcript_path` JSONL for richer fatigue signals: human turns, failed
#     tool calls, frantic cadence, and runs of the same repeated command. This
#     is what you see beside caveman's [CAVEMAN] badge inside Claude Code.
#
#   • Shell mode (fallback) — when nothing is piped in (e.g. run by hand, or no
#     transcript yet) we fall back to ~/.canary/canary-state, the file the
#     shell-prompt bird (canary.sh / canary.fish) refreshes on every command.
#
# Either way the bird's band, art and the optional multi-day "debt" are shared,
# so the prompt bird and the statusline bird tell the same story.
#
#   [CAVEMAN] ▗███▖ tired · 58m · 41t
#             ▐ - ▌>
#
# Knobs (export into Claude Code's env, or your shell's):
#   CANARY_DISABLED=1        bird sleeps (no output)
#   CANARY_MIN_SCORE=46      draw only at this score or higher (0 = always)
#   CANARY_SHOW_SCORE=1      append the raw numbers to the stat line
#   CANARY_ERR_WEIGHT=3      points per failed tool call (frustration)
#   CANARY_CADENCE_BASE=30   turns/hour considered "normal"; above it adds points
#   CANARY_REP_WEIGHT=2      points per extra repeat of the same command (stuck)
#   CANARY_DEBT_MAX=30       cap on yesterday's-fatigue carried into today
#   CANARY_HISTORY_FILE      where daily peaks live (default ~/.canary/history)
#   CANARY_DEAD_ABSOLUTE=1   show the dead bird at >90 always (default: only when
#                            today is worse than your own recent average)
#   CANARY_NIGHT_START=22 CANARY_NIGHT_END=7 CANARY_NIGHT_MULT=130  circadian penalty

[ "${CANARY_DISABLED:-0}" = "1" ] && exit 0

STATE="${CANARY_STATE_FILE:-$HOME/.canary/canary-state}"
HIST="${CANARY_HISTORY_FILE:-$HOME/.canary/history}"
ERR_WEIGHT=${CANARY_ERR_WEIGHT:-3}
CADENCE_BASE=${CANARY_CADENCE_BASE:-30}
REP_WEIGHT=${CANARY_REP_WEIGHT:-2}
DEBT_MAX=${CANARY_DEBT_MAX:-30}

# --- tiny JSON scraper (compact CC JSON only; no jq) -------------------------
# digits-only extraction doubles as terminal-escape-injection defense.
json_int() { printf '%s' "$1" | grep -o "\"$2\":[0-9]*" | head -1 | grep -o '[0-9]*'; }

# --- gather signals, per mode ------------------------------------------------
input=""
[ -t 0 ] || input=$(cat 2>/dev/null)   # CC pipes JSON; a TTY means nothing piped

min=0; turns=0; errors=0; reps=0; cadence=0
statname="t"   # label for the second stat (t=turns in CC mode, p=prompts in shell mode)

if printf '%s' "$input" | grep -q '"transcript_path"'; then
  # ---- Claude Code mode ----
  ms=$(json_int "$input" total_duration_ms); min=$(( ${ms:-0} / 60000 ))

  # transcript path: strip the JSON wrapper, refuse symlinks, require a real file
  line=$(printf '%s' "$input" | grep -o '"transcript_path":"[^"]*"' | head -1)
  tpath=${line#*:\"}; tpath=${tpath%\"}
  if [ -n "$tpath" ] && [ ! -L "$tpath" ] && [ -f "$tpath" ] && [ -r "$tpath" ]; then
    # human turns = user lines minus tool_result lines (CC wraps each tool result
    # as a "type":"user" line, so the raw count runs ~6x high).
    u=$(grep -c '"type":"user"' "$tpath" 2>/dev/null); u=${u:-0}
    tr=$(grep -c 'tool_result' "$tpath" 2>/dev/null); tr=${tr:-0}
    turns=$(( u - tr )); [ "$turns" -lt 0 ] && turns=0
    errors=$(grep -c '"is_error":true' "$tpath" 2>/dev/null); errors=${errors:-0}
    # longest run of the SAME command back-to-back (uniq = consecutive, so it
    # ignores interleaved hook noise). reps = that run length minus 1.
    reps=$(grep -o '"command":"[^"]*"' "$tpath" 2>/dev/null | uniq -c \
           | awk 'BEGIN{m=0}{if($1>m)m=$1}END{print m+0}')
    reps=$(( reps > 1 ? reps - 1 : 0 ))
  fi

  # cadence: turns/hour above CADENCE_BASE = frantic pace (reuses turns+min, no
  # timestamp parsing). ponytail: proxy of a proxy, but free given what we have.
  if [ "$min" -gt 0 ]; then
    rate=$(( turns * 60 / min ))
    cadence=$(( rate > CADENCE_BASE ? (rate - CADENCE_BASE) / 3 : 0 ))
  fi

  raw=$(( min / 3 + turns / 2 + errors * ERR_WEIGHT + cadence + reps * REP_WEIGHT ))
else
  # ---- shell-state fallback ----
  statname="p"
  [ -L "$STATE" ] && exit 0
  [ -f "$STATE" ] || exit 0
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
  if [ "$active" -ge 0 ]; then
    min=$(( active / 60 ))
  else
    min=$(( ($(date +%s) - ts_start) / 60 )); [ "$min" -lt 0 ] && min=0
  fi
  turns=$prompt_count
  raw=$(( min / 3 + prompt_count / 2 + avg_len / 10 ))
fi

# --- circadian penalty -------------------------------------------------------
ns=${CANARY_NIGHT_START:-22}; ne=${CANARY_NIGHT_END:-7}; nm=${CANARY_NIGHT_MULT:-130}
hour=$(( 10#$(date +%H) ))
if [ "$hour" -ge "$ns" ] || [ "$hour" -lt "$ne" ]; then
  raw=$(( raw * nm / 100 ))
fi
[ "$raw" -gt 100 ] && raw=100

# --- multi-day debt + personal baseline + night streak (from history) --------
# History lines: "<epoch-day> <peak>". Prior-day peaks decay by half per day of
# age and sum into today's debt (recovers over ~4-5 days), capped at DEBT_MAX.
today=$(( $(date +%s) / 86400 ))
debt=0; personal=0; nights=0
if [ -f "$HIST" ] && [ ! -L "$HIST" ]; then
  read -r debt personal nights <<EOF
$(awk -v today="$today" -v dmax="$DEBT_MAX" '
  { d[NR]=$1+0; p[NR]=$2+0; n=NR }
  END {
    debt=0; psum=0; pcnt=0;
    for (i=1;i<=n;i++) if (d[i] < today) {
      age = today - d[i]; v = p[i];
      for (k=0;k<age;k++) v = v/2;
      debt += v; psum += p[i]; pcnt++;
    }
    if (debt > dmax) debt = dmax;
    personal = (pcnt>0) ? int(psum/pcnt) : 0;
    nights=0; check=today-1; found=1;
    while (found) {
      found=0;
      for (i=1;i<=n;i++) if (d[i]==check && p[i]>=90) { found=1; break }
      if (found) { nights++; check-- }
    }
    printf "%d %d %d", int(debt), personal, nights;
  }' "$HIST")
EOF
fi
debt=${debt:-0}; personal=${personal:-0}; nights=${nights:-0}

score=$(( raw + debt )); [ "$score" -gt 100 ] && score=100
[ "$score" -gt 90 ] && nights=$(( nights + 1 ))   # today extends the streak

# --- persist today's RAW peak (pre-debt, no compounding); prune to 10 days ----
# ponytail: last-writer-wins across concurrent sessions; acceptable for a toy.
if [ ! -L "$HIST" ]; then
  mkdir -p "$(dirname "$HIST")" 2>/dev/null
  cur=$(awk -v d="$today" '$1==d{print $2; exit}' "$HIST" 2>/dev/null); cur=${cur:-0}
  new=$cur; [ "$raw" -gt "$cur" ] && new=$raw
  if [ "$new" != "$cur" ] || ! grep -q "^$today " "$HIST" 2>/dev/null; then
    tmp="$HIST.tmp.$$"
    { awk -v d="$today" '$1+0!=d' "$HIST" 2>/dev/null; echo "$today $new"; } \
      | sort -n -k1 | tail -10 > "$tmp" 2>/dev/null && mv "$tmp" "$HIST" 2>/dev/null
  fi
fi

# --- quiet threshold ---------------------------------------------------------
[ "$score" -lt "${CANARY_MIN_SCORE:-0}" ] && exit 0

# --- band → bird -------------------------------------------------------------
if   [ "$score" -le 20 ]; then state=fresh;  top='▗███▖';  eye='O'; beak='>'
elif [ "$score" -le 45 ]; then state=chirpy; top='▗███▖♪'; eye='^'; beak='>'
elif [ "$score" -le 70 ]; then state=tired;  top='▗███▖';  eye='-'; beak='>'
elif [ "$score" -le 90 ]; then state=worn;   top='▗▓▓▓▖';  eye='~'; beak='>'
else                           state=dead;   top='▗░░░▖';  eye='x'; beak='v'
fi

# Anti-habituation: a perma-grinder who is "dead" every night stops seeing it.
# Show the dead bird only when today is actually worse than your own recent
# average; otherwise calm it to worn. CANARY_DEAD_ABSOLUTE=1 restores fixed >90.
if [ "$state" = dead ] && [ "${CANARY_DEAD_ABSOLUTE:-0}" != "1" ] && [ "$raw" -le "$personal" ]; then
  state=worn; top='▗▓▓▓▖'; eye='~'; beak='>'
fi

# --- stat line + bird (Claude Code re-indents continuation lines by 2 spaces, so
# the two bird rows ride their own lines where they align; stats ride the badge) -
if [ "${CANARY_SHOW_SCORE:-0}" = "1" ]; then
  printf ' %s · %dm · %d%s · %de · d%d · %d\n' \
    "$state" "$min" "$turns" "$statname" "$errors" "$debt" "$score"
else
  printf ' %s · %dm · %d%s\n' "$state" "$min" "$turns" "$statname"
fi
printf '%s\n▐ %s ▌%s' "$top" "$eye" "$beak"

# Escalation: decoupled from the (now-calmed) face so the number still moves.
# Two or more consecutive days past your limit prints a line that keeps changing.
[ "$nights" -ge 2 ] && printf '\n✕ %d nights past your limit' "$nights"
