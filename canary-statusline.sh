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
#   CANARY_QUIET=1           no phrases; bird and note glyph only
#   CANARY_ASCII=1           ASCII substitutes for the tail and note glyphs
#   CANARY_RESERVE_COLS=0    cells to book on the bird's row for whatever else
#                            shares it (caveman's badge), so phrases don't wrap
#   CANARY_PHRASE_DIR        corpus root (default ~/.canary/phrases, then ./phrases)
#
# Subcommand:
#   canary-statusline.sh preview --state worn --note falling
#   canary-statusline.sh preview --state fresh --phrase "some candidate line"

[ "${CANARY_DISABLED:-0}" = "1" ] && exit 0

# --- `preview` subcommand ----------------------------------------------------
# Contributors who don't code cannot picture their line until they see it drawn.
# Same truncation and width path as the real render, no state written anywhere.
PREVIEW=0; PV_STATE=""; PV_NOTE=""; PV_PHRASE=""
if [ "${1:-}" = preview ]; then
  PREVIEW=1; shift
  while [ $# -gt 0 ]; do
    case $1 in
      --state)  PV_STATE=${2:-}; shift 2 ;;
      --note)   PV_NOTE=${2:-};  shift 2 ;;
      --phrase) PV_PHRASE=${2:-}; shift 2 ;;
      *) echo "usage: canary preview [--state fresh|chirpy|tired|worn|dead]" >&2
         echo "                      [--note rising|falling|steady|unknown] [--phrase TEXT]" >&2
         exit 2 ;;
    esac
  done
fi

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
[ "$PREVIEW" = 1 ] || [ -t 0 ] || input=$(cat 2>/dev/null)  # CC pipes JSON; a TTY means nothing piped

min=0; turns=0; errors=0; reps=0
statname="t"   # label for the second stat (t=turns in CC mode, p=prompts in shell mode)

if [ "$PREVIEW" = 1 ]; then
  raw=0
elif printf '%s' "$input" | grep -q '"transcript_path"'; then
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
now=$(date +%s); today=$(( now / 86400 ))
debt=0; personal=0; nights=0
if [ "$PREVIEW" != 1 ] && [ -f "$HIST" ] && [ ! -L "$HIST" ]; then
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
if [ "$PREVIEW" = 1 ]; then
  case "${PV_STATE:-fresh}" in
    fresh) score=10 ;; chirpy) score=35 ;; tired) score=60 ;;
    worn)  score=85 ;; dead)   score=95 ;;
    *) echo "canary: unknown state: $PV_STATE" >&2; exit 2 ;;
  esac
fi
[ "$score" -gt 90 ] && nights=$(( nights + 1 ))   # today extends the streak

# --- persist today's RAW peak (pre-debt, no compounding); prune to 10 days ----
# ponytail: last-writer-wins across concurrent sessions; acceptable for a toy.
if [ "$PREVIEW" != 1 ] && [ ! -L "$HIST" ]; then
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
[ "$PREVIEW" != 1 ] && [ "$score" -lt "${CANARY_MIN_SCORE:-0}" ] && exit 0

# --- band → bird -------------------------------------------------------------
if   [ "$score" -le 20 ]; then state=fresh;  top='▗███▖';  eye='O'; beak='>'
elif [ "$score" -le 45 ]; then state=chirpy; top='▗███▖';  eye='^'; beak='>'
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
if [ "$PREVIEW" != 1 ] && [ "$state" = dead ] && [ "${CANARY_DEAD_ABSOLUTE:-0}" != "1" ] \
   && [ "$raw" -le "$personal" ] && [ "$nights" -lt 2 ]; then
  state=worn; top='▗▓▓▓▖'; eye='~'; beak='>'
fi

# --- phrases (VOICE.md) ------------------------------------------------------
# The bird speaks only on a state TRANSITION, and even then most rolls produce
# nothing. Silence is the default and the right behaviour most of the time.
PHRASE_DIR="${CANARY_PHRASE_DIR:-}"
if [ -z "$PHRASE_DIR" ]; then
  for d in "$HOME/.canary/phrases" "$(dirname "$0")/phrases"; do
    [ -d "$d/en" ] && { PHRASE_DIR=$d; break; }
  done
fi
PSTATE="${CANARY_PHRASE_STATE:-$HOME/.canary/phrase-state}"
RECENT="${CANARY_RECENT_FILE:-$HOME/.canary/recent}"

# Corpus reader: drop `#` comments and blank lines, strip the optional trailing
# ` -- @handle` attribution, and drop anything non-printable — a corpus file is
# data on disk, and data on disk never gets to inject escapes into a status row.
phrase_lines() {
  [ $# -eq 0 ] && return 0
  cat "$@" 2>/dev/null \
    | sed -e 's/[[:space:]]*--[[:space:]]*@[^[:space:]]*[[:space:]]*$//' \
    | grep -v '^[[:space:]]*#' | grep -v '^[[:space:]]*$' | tr -cd '[:print:]\n'
}
phrase_draw() { awk -v s="$1" 'BEGIN{srand(s)}{a[NR]=$0}END{if(NR)print a[int(rand()*NR)+1]}'; }

# --- what the bird last knew -------------------------------------------------
prev_state=""; prev_score=""; prev_ts=0; ph_ts=0; ph_text=""
if [ "$PREVIEW" != 1 ] && [ -f "$PSTATE" ] && [ ! -L "$PSTATE" ]; then
  # sanitised with parameter expansion, not `tr` — this runs on every refresh
  # and five subshells here is five subshells too many.
  while IFS='=' read -r pk pv; do
    case "$pk" in
      state) prev_state=${pv//[!a-z]/} ;;
      score) prev_score=${pv//[!0-9]/} ;;
      ts)    prev_ts=${pv//[!0-9]/} ;;
      ph_ts) ph_ts=${pv//[!0-9]/} ;;
      ph)    ph_text=${pv//[^[:print:]]/} ;;
    esac
  done < "$PSTATE"
fi
prev_ts=${prev_ts:-0}; ph_ts=${ph_ts:-0}

# The note is the HUMAN's trend, not the score's: a score going up means the
# person is going down. `worn+rising` is someone pulling it back.
if   [ -z "$prev_score" ];           then note=unknown
elif [ "$score" -gt "$prev_score" ]; then note=falling
elif [ "$score" -lt "$prev_score" ]; then note=rising
else                                      note=steady
fi
[ -n "$PV_NOTE" ] && note=$PV_NOTE

gap=-1; [ "$prev_ts" -gt 0 ] && gap=$(( now - prev_ts ))
if   [ "$gap" -ge 86400 ]; then ret=days
elif [ "$gap" -ge 3600 ];  then ret=long
elif [ "$gap" -ge 180 ];   then ret=from-break
elif [ "$gap" -ge 60 ];    then ret=short
else                            ret=""
fi
on_break=0; [ "$gap" -ge 180 ] && on_break=1
late=0; { [ "$hour" -le 6 ] || [ "$min" -ge 240 ]; } && late=1

phrase_pick() {
  local root="$PHRASE_DIR/en" tier=common rare_ok=0 cand="" f
  # An ARRAY, not a space-separated string: an install path with a space in it
  # (~/Desktop/Progetti dev/...) word-split the pool and the bird went mute.
  local -a pool=()

  # dead bypasses the roll on purpose: its single line, and the silence that
  # follows it, is the loudest thing this tool says (VOICE.md rule 9).
  if [ "$state" = dead ]; then phrase_lines "$root/states/dead.txt"; return 0; fi

  # ~65% of eligible moments produce nothing at all (VOICE.md §4).
  [ $(( RANDOM % 100 )) -lt 65 ] && return 0

  # Rare is gated on HEALTH, never on time. An encounter system gated on
  # session length would pay you to keep working, which inverts the whole
  # tool — read VOICE.md §4 before touching this. Never at worn.
  case $state in
    fresh|chirpy) rare_ok=1 ;;
    tired) { [ "$on_break" = 1 ] || [ "$note" = rising ]; } && rare_ok=1 ;;
  esac

  if   [ "$rare_ok" = 1 ] && [ $(( RANDOM % 40 )) -eq 0 ]; then tier=rare
  elif [ $(( RANDOM % 8 )) -eq 0 ];                        then tier=uncommon
  fi

  case $tier in
    common)
      # state+note first, plain state as the fallback; an empty file keeps falling
      for f in "$root/states/$state+$note.txt" "$root/states/$state.txt"; do
        [ -f "$f" ] && [ -n "$(phrase_lines "$f")" ] && { pool=("$f"); break; }
      done
      if [ "$state" != worn ]; then
        [ -f "$root/notes/$note.txt" ] && pool+=("$root/notes/$note.txt")
        [ -n "$ret" ] && [ -f "$root/returns/$ret.txt" ] && pool+=("$root/returns/$ret.txt")
      fi
      [ "$late" = 1 ] && [ -f "$root/time/late.txt" ] && pool+=("$root/time/late.txt")
      ;;
    uncommon)
      # worn drops lore entirely (VOICE.md §6), so an uncommon roll there
      # degrades to silence rather than borrowing from a tier it can't have.
      [ "$state" = worn ] && return 0
      pool=("$root/lore/job.txt" "$root/lore/detector.txt") ;;
    rare)
      pool=("$root/lore/cage.txt" "$root/lore/facts.txt")
      [ "$state" != tired ] && pool+=("$root/worldly/outside.txt" "$root/worldly/culture.txt" "$root/worldly/subcultures.txt") ;;
  esac
  [ ${#pool[@]} -eq 0 ] && return 0

  # TODO(v0.8): true shuffle without replacement. The recent-queue below only
  # removes the visible defect — the same line twice running. It is not the
  # right algorithm, just the only one worth its persisted state in a script
  # Claude Code cancels mid-run.
  cand=$(phrase_lines "${pool[@]}" | phrase_draw "$RANDOM$$")
  if [ -n "$cand" ] && grep -Fxq -- "$cand" "$RECENT" 2>/dev/null; then
    cand=$(phrase_lines "${pool[@]}" | phrase_draw "$RANDOM$$$RANDOM")
  fi
  printf '%s' "$cand"
}

# Fit to the row. `tput cols` cannot see the terminal from inside a status line
# script, so COLUMNS is the only honest source. Word boundary, no ellipsis, and
# a stub shorter than ~12 chars is worse than saying nothing.
phrase_fit() {
  local text=$1 cols=${COLUMNS:-0} avail slice cut
  [ "$cols" -le 0 ] && cols=80
  # 2 cells for Claude Code's continuation indent, 10 for "▐ O ▌>  ⌐ ".
  avail=$(( cols - 12 - ${CANARY_RESERVE_COLS:-0} ))
  [ "$avail" -lt 12 ] && return 0
  [ "${#text}" -le "$avail" ] && { printf '%s' "$text"; return 0; }
  slice=${text:0:$((avail + 1))}
  case $slice in *' '*) cut=${slice% *} ;; *) return 0 ;; esac
  [ "${#cut}" -lt 12 ] && return 0
  printf '%s' "$cut"
}

phrase=""; drew=0
if [ "${CANARY_QUIET:-0}" != "1" ] && [ -n "$PHRASE_DIR" ]; then
  if [ -n "$PV_PHRASE" ]; then
    phrase=$PV_PHRASE
  elif [ "$PREVIEW" = 1 ]; then
    for f in "$PHRASE_DIR/en/states/$state+$note.txt" "$PHRASE_DIR/en/states/$state.txt"; do
      [ -f "$f" ] && phrase=$(phrase_lines "$f" | phrase_draw "$RANDOM$$")
      [ -n "$phrase" ] && break
    done
  elif [ "$state" != "$prev_state" ]; then
    phrase=$(phrase_pick); drew=1; ph_ts=$now
  elif [ -n "$ph_text" ] && [ $(( now - ph_ts )) -le 60 ]; then
    phrase=$ph_text          # auto-mute: the same line holds ~60s, then quiet
  else
    ph_text=""
  fi
fi

if [ "$drew" = 1 ] && [ -n "$phrase" ] && [ ! -L "$RECENT" ]; then
  mkdir -p "$(dirname "$RECENT")" 2>/dev/null
  { cat "$RECENT" 2>/dev/null; printf '%s\n' "$phrase"; } | tail -10 > "$RECENT.tmp.$$" \
    && mv "$RECENT.tmp.$$" "$RECENT" 2>/dev/null
fi

if [ "$PREVIEW" != 1 ] && [ ! -L "$PSTATE" ]; then
  mkdir -p "$(dirname "$PSTATE")" 2>/dev/null
  printf 'state=%s\nscore=%d\nts=%d\nph_ts=%d\nph=%s\n' \
    "$state" "$score" "$now" "$ph_ts" "$phrase" > "$PSTATE.tmp.$$" \
    && mv "$PSTATE.tmp.$$" "$PSTATE" 2>/dev/null
fi

# The balloon and the note share the one slot right of the bird: note when
# silent, text when speaking, nothing at all when dead and done talking.
if [ "${CANARY_ASCII:-0}" = "1" ]; then p_tail='-'; p_note='.'
else                                    p_tail='⌐'; p_note='♪'
fi
slot=""
if [ -n "$phrase" ]; then
  fit=$(phrase_fit "$phrase")
  [ -n "$fit" ] && slot="  $p_tail $fit"
fi
[ -z "$slot" ] && [ "$state" != dead ] && slot="  $p_note"

# --- stat line + bird (Claude Code re-indents continuation lines by 2 spaces, so
# the two bird rows ride their own lines where they align; stats ride the badge) -
if [ "${CANARY_SHOW_SCORE:-0}" = "1" ]; then
  printf ' %s · %dm · %d%s · %de · d%d · %d\n' \
    "$state" "$min" "$turns" "$statname" "$errors" "$debt" "$score"
else
  printf ' %s · %dm · %d%s\n' "$state" "$min" "$turns" "$statname"
fi
printf '%s\n▐ %s ▌%s%s' "$top" "$eye" "$beak" "$slot"

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
