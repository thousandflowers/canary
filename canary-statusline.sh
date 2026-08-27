#!/usr/bin/env bash
# canary-statusline.sh — the fatigue bird for Claude Code's statusLine.
#
# DUAL-MODE (zero runtime deps — pure shell + grep/awk, no jq, no ANSI color):
#
#   • Claude Code mode  — Claude Code pipes its session JSON on stdin every
#     refresh. We read `cost.total_duration_ms` for session minutes and walk the
#     `transcript_path` JSONL for richer fatigue signals: human turns, failed
#     tool calls, and runs of the same repeated command. This
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
#   CANARY_REP_WEIGHT=2      points per extra repeat of the same command (stuck)
#   CANARY_DEBT_MAX=30       cap on yesterday's-fatigue carried into today
#   CANARY_HISTORY_FILE      where daily peaks live (default ~/.canary/history)
#   CANARY_DEAD_ABSOLUTE=1   show the dead bird at >90 always (default: only when
#                            today is worse than your own recent average)
#   CANARY_NIGHT_MULT=150    multiplier at the bottom of the circadian trough
#                            (02:00-04:00); scales the whole curve, 100 = off

[ "${CANARY_DISABLED:-0}" = "1" ] && exit 0

STATE="${CANARY_STATE_FILE:-$HOME/.canary/canary-state}"
HIST="${CANARY_HISTORY_FILE:-$HOME/.canary/history}"
ERR_WEIGHT=${CANARY_ERR_WEIGHT:-3}
REP_WEIGHT=${CANARY_REP_WEIGHT:-2}
DEBT_MAX=${CANARY_DEBT_MAX:-30}

# --- tiny JSON scraper (compact CC JSON only; no jq) -------------------------
# digits-only extraction doubles as terminal-escape-injection defense.
json_int() { printf '%s' "$1" | grep -o "\"$2\":[0-9]*" | head -1 | grep -o '[0-9]*'; }

# --- active minutes -> points ------------------------------------------------
# Concave, not linear: the vigilance decrement is front-loaded (roughly half of
# it inside the first ~15 min, reaction times climbing past ~30 min, costs
# steepening after ~60 min) and then flattens. Identical to canary.sh, because
# the prompt bird and this one must tell the same story.
#   15m->7  30m->14  1h->26  2h->43  3h->55  5h->72  8h->86  12h->97
time_points() { echo $(( $1 * 130 / ($1 + 240) )); }

# --- time-of-day: percentage points added, at full amplitude -----------------
# Circadian nadir 02:00-06:00, deepest 02:00-04:00; a real (circadian, not
# dietary) post-lunch dip 13:00-16:00; and 17:00-21:00 is the evening wake
# maintenance zone, where alertness is genuinely high, so nothing is added.
circadian_excess() {
  case $1 in
    2|3|4)          echo 50 ;;
    5|6)            echo 40 ;;
    0|1)            echo 25 ;;
    7|13|14|15|23)  echo 15 ;;
    16|22)          echo 5 ;;
    *)              echo 0 ;;
  esac
}

# --- gather signals, per mode ------------------------------------------------
input=""
[ -t 0 ] || input=$(cat 2>/dev/null)   # CC pipes JSON; a TTY means nothing piped

min=0; turns=0; errors=0; reps=0
statname="t"   # label for the second stat (t=turns in CC mode, p=prompts in shell mode)

if printf '%s' "$input" | grep -q '"transcript_path"'; then
  # ---- Claude Code mode ----
  ms=$(json_int "$input" total_duration_ms); min=$(( ${ms:-0} / 60000 ))

  # transcript path: strip the JSON wrapper, refuse symlinks, require a real file
  line=$(printf '%s' "$input" | grep -o '"transcript_path":"[^"]*"' | head -1)
  tpath=${line#*:\"}; tpath=${tpath%\"}
  if [ -n "$tpath" ] && [ ! -L "$tpath" ] && [ -f "$tpath" ] && [ -r "$tpath" ]; then
    # ponytail: scan only the last ~2MB. On long sessions the transcript grows to
    # tens of MB; grepping the whole file ~4x per refresh blew Claude Code's
    # statusline timeout, so NOTHING rendered — the bird went invisible in long
    # sessions (the actual bug). `tail -c` caps the work at ~0.3s regardless of
    # file size. Older lines can't change the band anyway (a session that long is
    # already maxed on `min`), so turns/errors/reps reflect recent activity —
    # which is what fatigue should weight. (tail -n is wrong: N huge JSONL lines
    # are still tens of MB.)
    buf=$(tail -c 2000000 "$tpath" 2>/dev/null)
    # human turns = "type":"user" lines that AREN'T tool results (CC wraps each
    # tool result as its own "type":"user" line). Excluding tool_result lines
    # directly is robust; the old "users - grep tool_result" underflowed to 0.
    turns=$(grep '"type":"user"' <<<"$buf" | grep -vc 'tool_result'); turns=${turns:-0}
    errors=$(grep -c '"is_error":true' <<<"$buf"); errors=${errors:-0}
    # longest run of the SAME command back-to-back (uniq = consecutive). reps =
    # run length minus 1, capped at 5 so a polling/retry loop can't alone kill it.
    reps=$(grep -o '"command":"[^"]*"' <<<"$buf" | uniq -c \
           | awk 'BEGIN{m=0}{if($1>m)m=$1}END{print m+0}')
    reps=$(( reps > 1 ? reps - 1 : 0 )); [ "$reps" -gt 5 ] && reps=5
  fi

  # ponytail: no cadence term. turns/hour divided by a tiny early-session `min`
  # exploded into noise, and "slower pace = less tired" is backwards for a
  # fatigue meter — it made the score non-monotonic in time (bird got HEALTHIER
  # as minutes passed). errors + reps already capture frantic/stuck. Fatigue now
  # only ever climbs within a session.
  # errors and reps are the best-evidenced terms canary has: mental fatigue
  # reliably increases error rates AND perseveration — repeating an action that
  # is not working — which is exactly what `reps` counts. The shell-mode
  # fallback below has neither signal, which makes this the better instrument.
  raw=$(( $(time_points "$min") + turns / 2 + errors * ERR_WEIGHT + reps * REP_WEIGHT ))
else
  # ---- shell-state fallback ----
  statname="p"
  [ -L "$STATE" ] && exit 0
  [ -f "$STATE" ] || exit 0
  prompt_count=0; avg_len=0; active=0
  while IFS='=' read -r k v; do
    v=$(printf '%s' "$v" | tr -cd '0-9')
    case "$k" in
      prompt_count)   prompt_count=${v:-0} ;;
      avg_prompt_len) avg_len=${v:-0} ;;
      active_seconds) active=${v:-0} ;;
    esac
  done < "$STATE"
  min=$(( active / 60 ))
  turns=$prompt_count
  raw=$(( $(time_points "$min") + prompt_count / 2 + avg_len / 10 ))
fi

# --- time of day -------------------------------------------------------------
# CANARY_NIGHT_MULT is the multiplier at the bottom of the trough (150 = x1.5 at
# 02:00-04:00) and scales the whole curve, so 100 switches it off.
nm=${CANARY_NIGHT_MULT:-150}
hour=$(( 10#$(date +%H) ))
excess=$(( $(circadian_excess "$hour") * (nm - 100) / 50 ))
[ "$excess" -gt 0 ] && raw=$(( raw * (100 + excess) / 100 ))
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

# Anti-habituation: a perma-grinder who is "dead" every night stops seeing it,
# so a bird that is always dead carries no information. Calm it to worn when
# today is no worse than your own recent average.
#
# But NOT during a streak. The core finding on chronic sleep restriction is that
# deficits accumulate to severe levels "without full awareness of the affected
# individuals" — the person several days deep is precisely the one who cannot
# feel it, and muting the alarm for them inverts the point of the bird. So the
# demotion is relief for a single bad day, never for an accumulating one.
# CANARY_DEAD_ABSOLUTE=1 restores a fixed >90.
if [ "$state" = dead ] && [ "${CANARY_DEAD_ABSOLUTE:-0}" != "1" ] \
   && [ "$raw" -le "$personal" ] && [ "$nights" -lt 2 ]; then
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
if [ "$nights" -ge 2 ]; then
  printf '\n✕ %d nights past your limit' "$nights"
fi

# Exit 0 explicitly. As an `&&` one-liner the test above was the script's last
# command, so every ordinary render — the overwhelmingly common case, nights < 2
# — exited 1. Claude Code chains status line commands with `;`, so canary's
# status leaked out as the whole chain's, and `brew test` fails on a non-zero
# exit from shell_output.
exit 0
