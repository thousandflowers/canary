#!/usr/bin/env bash
# canary-statusline.sh — a pixel-art bird for Claude Code's statusLine that
# wilts the longer/harder your sessions run, across days. For fun, not science.
#
# Claude Code pipes a status JSON on stdin every refresh; this script reads it,
# scores fatigue, and prints the bird. Two lines:
#
#   ▗███▖ tired · 58m · 41t
#   ▐ - ▌>
#
# Zero deps: pure shell + grep/awk. No jq, no color, no API calls.
#
# Fatigue (capped 0-100), optional night penalty, plus a multi-day debt:
#   score = minutes/3 + turns/2 + errors×3 + cadence + reps×2   (today's raw)
#         + carried-over debt from recent days (rest pays it down)
#   minutes  = session wall-clock (cost.total_duration_ms)
#   turns    = your messages in the transcript
#   errors   = failed tool calls (is_error:true) — frustration
#   cadence  = how frantic the pace is (turns per hour over a baseline)
#   reps     = longest run of the same command back-to-back — stuck in a loop
# Still a proxy, not a doctor — but it now tells a smooth run from a slog.
#
# Knobs (export into Claude Code's environment):
#   CANARY_DISABLED=1       hide the bird
#   CANARY_MIN_SCORE=N      only draw at/above this score (0 = always; 46 = tired+)
#   CANARY_SHOW_SCORE=1     append the numeric breakdown
#   CANARY_ERR_WEIGHT=3     score per errored tool call (0 = ignore)
#   CANARY_CADENCE_BASE=30  turns/hour above which the pace counts as frantic
#   CANARY_REP_WEIGHT=2     score per extra repeat in the longest same-command run
#   CANARY_DEBT_MAX=30      cap on carried-over multi-day fatigue
#   CANARY_HISTORY_FILE     where daily peaks live (default ~/.canary/history)
#   CANARY_NIGHT_START=22 CANARY_NIGHT_END=7 CANARY_NIGHT_MULT=130  (100 = off)

[ -n "${CANARY_DISABLED:-}" ] && exit 0

json=$(cat)   # the whole status blob Claude Code sends on stdin

# --- session minutes from total_duration_ms (digits only = injection-safe) ---
dur_ms=$(printf '%s' "$json" | grep -oE '"total_duration_ms":[0-9]+' | head -1 | grep -oE '[0-9]+$')
[ -n "$dur_ms" ] || dur_ms=0
min=$(( dur_ms / 60000 ))

# --- transcript-derived signals: turns, errors, repetition ------------------
tpath=$(printf '%s' "$json" | grep -oE '"transcript_path":"[^"]*"' | head -1 | sed 's/.*:"//; s/"$//')
turns=0
errors=0
reps=0
if [ -n "$tpath" ] && [ -f "$tpath" ] && [ ! -L "$tpath" ]; then
  # human turns = user-type lines minus the tool_result lines Claude Code also
  # records as "type":"user" (ponytail: line-grep, not a real JSONL parse).
  turns=$(grep '"type":"user"' "$tpath" 2>/dev/null | grep -vc 'tool_result')
  [ -n "$turns" ] || turns=0
  # frustration: failed tool calls (a non-zero exit shows up as is_error:true)
  errors=$(grep -c '"is_error":true' "$tpath" 2>/dev/null)
  [ -n "$errors" ] || errors=0
  # stuck-in-a-loop: longest run of the SAME command back-to-back. Only count
  # real tool_use lines — Claude Code logs its hook machinery's commands on
  # hook_success/attachment lines, which would otherwise swamp the signal. This
  # filters by message *type*, not a hardcoded list of tool names.
  maxrun=$(grep '"type":"tool_use"' "$tpath" 2>/dev/null | grep -oE '"command":"[^"]*"' | uniq -c \
           | awk 'BEGIN{m=0}{if($1>m)m=$1}END{print m}')
  [ -n "$maxrun" ] || maxrun=0
  [ "$maxrun" -gt 1 ] && reps=$(( maxrun - 1 ))
fi

# --- cadence: frantic pace = many turns crammed into little time ------------
cadence=0
if [ "$min" -gt 0 ]; then
  rate=$(( turns * 60 / min ))                 # turns per hour
  base=${CANARY_CADENCE_BASE:-30}
  [ "$rate" -gt "$base" ] && cadence=$(( (rate - base) / 3 ))
fi

# --- today's raw fatigue ----------------------------------------------------
raw=$(( min / 3 + turns / 2 + errors * ${CANARY_ERR_WEIGHT:-3} + cadence + reps * ${CANARY_REP_WEIGHT:-2} ))

# --- circadian penalty (defaults assume a daytime schedule) -----------------
ns=${CANARY_NIGHT_START:-22}; ne=${CANARY_NIGHT_END:-7}; nm=${CANARY_NIGHT_MULT:-130}
hour=$(( 10#$(date +%H) ))
if [ "$hour" -ge "$ns" ] || [ "$hour" -lt "$ne" ]; then
  raw=$(( raw * nm / 100 ))
fi
[ "$raw" -gt 100 ] && raw=100

# --- multi-day debt: a hard week carries over, rest pays it down ------------
# history lines: "<whole-days-since-epoch> <that-day's-peak>". Whole-day integers
# dodge all date parsing — age is just subtraction.
HIST="${CANARY_HISTORY_FILE:-$HOME/.canary/history}"
today_d=$(( $(date +%s) / 86400 ))
carry=0
prev_today=0
prior_sum=0       # sum of prior-day peaks, for your personal "normal"
prior_n=0
dead_days=""      # day numbers whose peak hit the dead band, for streak detection
if [ -f "$HIST" ] && [ ! -L "$HIST" ]; then
  while read -r d s; do
    case "$d" in ''|\#*) continue ;; esac
    d=$(printf '%s' "$d" | tr -cd '0-9'); s=$(printf '%s' "$s" | tr -cd '0-9')
    [ -n "$d" ] && [ -n "$s" ] || continue
    if [ "$d" = "$today_d" ]; then
      prev_today=$s
    elif [ "$d" -lt "$today_d" ]; then
      # halve the old peak once per day of age -> ~gone after 4-5 days = recovery
      age=$(( today_d - d )); w=$s; i=0
      while [ "$i" -lt "$age" ] && [ "$w" -gt 0 ]; do w=$(( w / 2 )); i=$(( i + 1 )); done
      carry=$(( carry + w ))
      prior_sum=$(( prior_sum + s )); prior_n=$(( prior_n + 1 ))
      [ "$s" -ge 90 ] && dead_days="$dead_days $d"
    fi
  done < "$HIST"
fi
dmax=${CANARY_DEBT_MAX:-30}
[ "$carry" -gt "$dmax" ] && carry=$dmax

# anti-habituation: your recent "normal" peak + how many days running you've
# been in the dead band (counting back from yesterday)
personal=0; [ "$prior_n" -gt 0 ] && personal=$(( prior_sum / prior_n ))
streak=0; check=$(( today_d - 1 ))
while case " $dead_days " in *" $check "*) true ;; *) false ;; esac; do
  streak=$(( streak + 1 )); check=$(( check - 1 ))
done

# record today's peak (store RAW, pre-carry, so debt never compounds). Only
# rewrite when the peak actually grows — keeps refresh-time writes rare.
peak=$raw; [ "$prev_today" -gt "$peak" ] && peak=$prev_today
if [ "$peak" -gt "$prev_today" ] && mkdir -p "${HIST%/*}" 2>/dev/null && [ ! -L "$HIST" ]; then
  # ponytail: last-writer-wins if two CC windows refresh at once — fine for a toy.
  tmp=$(mktemp 2>/dev/null || echo "$HIST.tmp")
  {
    printf '%s %s\n' "$today_d" "$peak"
    if [ -f "$HIST" ]; then
      while read -r d s; do
        case "$d" in ''|\#*) continue ;; esac
        d=$(printf '%s' "$d" | tr -cd '0-9'); s=$(printf '%s' "$s" | tr -cd '0-9')
        [ -n "$d" ] && [ -n "$s" ] || continue
        [ "$d" -lt "$today_d" ] && [ "$(( today_d - d ))" -le 10 ] && printf '%s %s\n' "$d" "$s"
      done < "$HIST"
    fi
  } > "$tmp" 2>/dev/null
  mv "$tmp" "$HIST" 2>/dev/null || rm -f "$tmp" 2>/dev/null
fi

score=$(( raw + carry ))
[ "$score" -gt 100 ] && score=100

# optional quiet threshold — show nothing below it
[ "$score" -lt "${CANARY_MIN_SCORE:-0}" ] && exit 0

if   [ "$score" -le 20 ]; then state=fresh;  top='▗███▖';  eye='O'; beak='>'
elif [ "$score" -le 45 ]; then state=chirpy; top='▗███▖♪'; eye='^'; beak='>'
elif [ "$score" -le 70 ]; then state=tired;  top='▗███▖';  eye='-'; beak='>'
elif [ "$score" -le 90 ]; then state=worn;   top='▗▓▓▓▖';  eye='~'; beak='>'
else
  # the dead FACE shows only on a day genuinely worse than YOUR recent norm —
  # a perma-grind dead every night is wallpaper, not a nudge. Set
  # CANARY_DEAD_ABSOLUTE=1 for the old fixed >90 face.
  if [ -z "${CANARY_DEAD_ABSOLUTE:-}" ] && [ "$raw" -le "$personal" ]; then
    state=worn; top='▗▓▓▓▖'; eye='~'; beak='>'
  else
    state=dead; top='▗░░░▖'; eye='x'; beak='v'
  fi
fi

# persistent-grind warning, decoupled from the face: days running (incl. today)
# in the dead band. A line whose number changes can't fade into wallpaper.
nights=0; [ "$score" -gt 90 ] && nights=$(( streak + 1 ))

# Claude Code re-indents continuation lines by 2 spaces, so the two bird rows
# sit on their own lines (aligned) while the stats ride the first line.
if [ -n "${CANARY_SHOW_SCORE:-}" ]; then
  printf ' %s · %dm · %dt · %de · d%d · %d\n%s\n▐ %s ▌%s' \
    "$state" "$min" "$turns" "$errors" "$carry" "$score" "$top" "$eye" "$beak"
else
  printf ' %s · %dm · %dt\n%s\n▐ %s ▌%s' "$state" "$min" "$turns" "$top" "$eye" "$beak"
fi

# escalate a dead streak — a changing message can't fade into wallpaper the way
# the same dead bird every night would
if [ "${nights:-0}" -ge 2 ]; then
  printf '\n  ✕ %d nights past your limit — close the laptop.' "$nights"
fi
