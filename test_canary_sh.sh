#!/usr/bin/env bash
# Self-check for canary.sh — the shell-prompt bird. No framework, just asserts.
#   bash test_canary_sh.sh   # exits non-zero on any failure
#
# The circadian penalty is pinned off (CANARY_NIGHT_MULT=100) everywhere except
# the one test that exercises it, so scores don't depend on the time of day.
#
# SC2016: single quotes are deliberate throughout. Every probe body is a *string*
# handed to a child `bash -c`; expanding it here would evaluate canary's
# variables in the test's own shell instead of the one under test.
# shellcheck disable=SC2016

set -u
HERE=$(cd "$(dirname "$0")" && pwd)
SCRIPT="$HERE/canary.sh"
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

fails=0
assert_has()   { case "$2" in *"$3"*) ;; *) echo "FAIL [$1]: expected to contain '$3'"; echo "  got: $(printf '%s' "$2" | tr '\n' '|')"; fails=$((fails+1));; esac; }
assert_eq()    { [ "$2" = "$3" ] && return; echo "FAIL [$1]: expected '$3', got '$2'"; fails=$((fails+1)); }
assert_empty() { [ -z "$2" ] && return; echo "FAIL [$1]: expected empty, got: $2"; fails=$((fails+1)); }

# Run shell code in a fresh bash with canary.sh sourced. The DEBUG trap is
# removed unless the caller asks for it, so a test's own commands aren't
# recorded as if the user had typed them.
run() { # run [KEEP_TRAP] <env assignments...> -- <code>
  local keep=""
  [ "${1:-}" = "KEEP_TRAP" ] && { keep=1; shift; }
  local env_args=()
  while [ "$#" -gt 0 ] && [ "$1" != "--" ]; do env_args+=("$1"); shift; done
  shift  # drop --
  local body=$1
  local untrap="trap - DEBUG; PROMPT_COMMAND="
  [ -n "$keep" ] && untrap=":"
  # ${a[@]+"${a[@]}"}: bash 3.2 (stock macOS) treats an empty array as unbound
  # under `set -u`. This expands to nothing when env_args is empty.
  env CANARY_NIGHT_MULT=100 CANARY_STATE_FILE="$TMP/state.$RANDOM" \
      ${env_args[@]+"${env_args[@]}"} bash -c ". '$SCRIPT'; $untrap; $body" 2>&1
}

# --- 1. every band renders its own bird, at both edges of its range ----------
while read -r score want_name want_eye; do
  out=$(run -- "_canary_render $score show")
  assert_has "band-$score-name" "$out" "[$want_name $score]"
  assert_has "band-$score-eye"  "$out" "$want_eye"
done <<'BANDS'
0 fresh ▐ O ▌>
20 fresh ▐ O ▌>
21 chirpy ▐ ^ ▌>
45 chirpy ▐ ^ ▌>
46 tired ▐ - ▌>
70 tired ▐ - ▌>
71 worn ▐ ~ ▌>
90 worn ▐ ~ ▌>
91 dead ░ x ▌v
100 dead ░ x ▌v
BANDS

# the dead bird nags; no other band does
assert_has "dead-nag" "$(run -- '_canary_render 95 show')" 'CANARY_RESET=1'
case "$(run -- '_canary_render 90 show')" in
  *CANARY_RESET=1*) echo "FAIL [worn-no-nag]: worn should not nag"; fails=$((fails+1));;
esac

# --- 2. score formula: time_points(min) + count/2 + avglen/10 ----------------
# 3600s active = 60min -> 26 ; 40 prompts -> 20 ; avg len 50 -> 5 ; total 51
out=$(run CANARY_ACTIVE_SECONDS=3600 CANARY_PROMPT_COUNT=40 CANARY_LEN_SUM=2000 -- '_canary_score')
assert_eq "score-formula" "$out" "51"

# the time curve is concave and front-loaded: half the vigilance decrement
# lands in the first ~15 min, costs steepen past ~60 min, then it flattens.
# These are the reference points documented in canary.sh.
while read -r mins want; do
  assert_eq "time-curve-${mins}m" "$(run -- "_canary_time_points $mins")" "$want"
done <<'CURVE'
0 0
15 7
30 14
60 26
120 43
180 55
300 72
480 86
720 97
CURVE

# concave means each successive hour adds less than the one before it
out=$(run -- 'a=$(_canary_time_points 60); b=$(_canary_time_points 120); c=$(_canary_time_points 180); [ $((b-a)) -lt $a ] && [ $((c-b)) -lt $((b-a)) ] && echo concave || echo LINEAR')
assert_eq "time-curve-concave" "$out" "concave"

# ...and it never saturates early: 5h must still be below the cap, or the dead
# bird becomes wallpaper on any long day (the flaw in the old linear min/3).
out=$(run -- '[ "$(_canary_time_points 300)" -lt 100 ] && echo below || echo PINNED')
assert_eq "time-curve-no-early-cap" "$out" "below"

# score is clamped to 100, never above
out=$(run CANARY_ACTIVE_SECONDS=999999 CANARY_PROMPT_COUNT=9999 -- '_canary_score')
assert_eq "score-clamped" "$out" "100"

# an untouched session scores 0
assert_eq "score-zero" "$(run -- '_canary_score')" "0"

# --- 3. mean command length ---------------------------------------------------
assert_eq "avg-empty" "$(run -- '_canary_avg')" "0"
assert_eq "avg-mean"  "$(run CANARY_PROMPT_COUNT=3 CANARY_LEN_SUM=60 -- '_canary_avg')" "20"

# the sum tracks every recorded command, and the denominator is PROMPT_COUNT —
# the two must stay in step or the average drifts
out=$(run -- 'i=0; while [ $i -lt 25 ]; do _canary_record "xx"; i=$((i+1)); done; echo "$CANARY_LEN_SUM/$CANARY_PROMPT_COUNT=$(_canary_avg)"')
assert_eq "avg-running-sum" "$out" "50/25=2"

# ...and an ignored empty command must move neither
out=$(run -- '_canary_record "abcd"; _canary_record ""; echo "$CANARY_LEN_SUM/$CANARY_PROMPT_COUNT"')
assert_eq "avg-empty-not-summed" "$out" "4/1"

# --- 4. idle gaps don't age the bird -----------------------------------------
# a gap larger than the threshold is a break: active time must not grow
out=$(run CANARY_IDLE_THRESHOLD=1 -- 'CANARY_LAST_ACTIVE=$(( $(date +%s) - 3600 )); _canary_record "ls"; echo $CANARY_ACTIVE_SECONDS')
assert_eq "idle-gap-ignored" "$out" "0"

# a gap inside the threshold is work: it accrues
out=$(run CANARY_IDLE_THRESHOLD=99999 -- 'CANARY_LAST_ACTIVE=$(( $(date +%s) - 600 )); _canary_record "ls"; echo $CANARY_ACTIVE_SECONDS')
assert_eq "active-gap-accrued" "$out" "600"

# --- 5. empty command line (bare Enter) is not a prompt ----------------------
out=$(run -- '_canary_record ""; echo $CANARY_PROMPT_COUNT')
assert_eq "empty-not-counted" "$out" "0"

# --- 6. time of day ----------------------------------------------------------
# The old flat "penalty from 22:00 to 07:00" is gone. The curve now follows the
# circadian literature: nadir 02:00-06:00 (deepest 02:00-04:00), a post-lunch
# dip 13:00-16:00, and nothing added 17:00-21:00 — the evening wake maintenance
# zone, where alertness is genuinely high and the old rule was penalising you.
while read -r hour want; do
  assert_eq "circadian-${hour}h" "$(run -- "_canary_circadian_excess $hour")" "$want"
done <<'HOURS'
0 25
1 25
2 50
3 50
4 50
5 40
6 40
7 15
9 0
12 0
13 15
15 15
16 5
20 0
21 0
22 5
23 15
HOURS

# the trough must be the maximum, and the morning peak and evening wake
# maintenance zone must both be free
out=$(run -- 'mx=0; for h in 0 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20 21 22 23; do v=$(_canary_circadian_excess $h); [ "$v" -gt "$mx" ] && mx=$v; done; echo "$mx/$(_canary_circadian_excess 3)/$(_canary_circadian_excess 10)/$(_canary_circadian_excess 20)"')
assert_eq "circadian-shape" "$out" "50/50/0/0"

# CANARY_NIGHT_MULT=100 switches the whole time-of-day adjustment off, at any hour
assert_eq "circadian-disabled" "$(run CANARY_ACTIVE_SECONDS=3600 CANARY_NIGHT_MULT=100 -- '_canary_score')" "26"
# ...and a bigger multiplier can only ever raise the score, never lower it
out=$(run CANARY_ACTIVE_SECONDS=3600 CANARY_NIGHT_MULT=150 -- '[ "$(_canary_score)" -ge 26 ] && echo ge || echo LOWER')
assert_eq "circadian-monotone" "$out" "ge"

# --- 7. quiet threshold + disabled -------------------------------------------
assert_empty "min-score-quiet" "$(run CANARY_MIN_SCORE=50 -- '_canary_precmd')"
assert_has   "min-score-loud"  "$(run CANARY_MIN_SCORE=0 -- '_canary_precmd')" '▐ O ▌>'
# CANARY_DISABLED short-circuits at source time, so nothing is defined at all
assert_empty "disabled" "$(run CANARY_DISABLED=1 -- '_canary_precmd 2>/dev/null')"

# --- 8. state file: the fields the statusline reads --------------------------
S="$TMP/state.explicit"
run CANARY_STATE_FILE="$S" -- '_canary_record "hello"; _canary_record "worldly"' >/dev/null
assert_has "state-count"  "$(cat "$S")" "prompt_count=2"
assert_has "state-avg"    "$(cat "$S")" "avg_prompt_len=6"   # (5+7)/2
assert_has "state-active" "$(cat "$S")" "active_seconds="

# the statusline must be able to consume what the prompt bird just wrote
out=$(env CANARY_NIGHT_MULT=100 CANARY_STATE_FILE="$S" CANARY_HISTORY_FILE="$TMP/h.$RANDOM" \
      bash "$HERE/canary-statusline.sh" </dev/null)
assert_has "state-roundtrip" "$out" "2p"

# --- 9. reset wipes the session ----------------------------------------------
out=$(run -- '_canary_record "ls"; CANARY_RESET=1; _canary_precmd >/dev/null; echo "$CANARY_PROMPT_COUNT/$CANARY_LEN_SUM/$CANARY_ACTIVE_SECONDS/${CANARY_RESET:-unset}"')
assert_eq "reset" "$out" "0/0/0/unset"

# --- 10. the `canary` command ------------------------------------------------
assert_has "cmd-status"  "$(run -- 'canary')"        '▐ O ▌>'
assert_has "cmd-score"   "$(run -- 'canary score')"  '0'
assert_has "cmd-reset"   "$(run -- 'canary reset')"  'canary: reset'
assert_has "cmd-help"    "$(run -- 'canary --help')" 'usage: canary'
assert_has "cmd-off"     "$(run -- 'canary off')"    'canary: off'
assert_has "cmd-unknown" "$(run -- 'canary bogus')"  'unknown command: bogus'
out=$(run -- 'canary bogus >/dev/null 2>&1; echo "exit=$?"')
assert_eq "cmd-unknown-exit" "$out" "exit=1"

# --- 11. re-sourcing is a no-op (load guard) ---------------------------------
out=$(run -- '_canary_record "ls"; . "'"$SCRIPT"'"; trap - DEBUG; echo $CANARY_PROMPT_COUNT')
assert_eq "load-guard" "$out" "1"

# --- 12. no footprint in the user's shell ------------------------------------
# REGRESSION GUARD: canary.sh once assigned bare `cmd`, `len`, `score`, `name`,
# `now`, `gap`, `min`, `s`, `hour`, `n`, `sum`, `x` — clobbering those names in
# the *interactive* shell on every prompt. In bash the DEBUG trap even parked
# the full text of the last command in $cmd. Every internal must be _cy_*.
probe='cmd=K len=K n=K sum=K x=K min=K s=K hour=K score=K name=K now=K gap=K force=K l1=K l2=K avglen=K
. "'"$SCRIPT"'"; trap - DEBUG; PROMPT_COMMAND=
_canary_record "echo hello"; _canary_precmd >/dev/null; _canary_render 55 show >/dev/null; canary status >/dev/null
bad=""
for v in cmd len n sum x min s hour score name now gap force l1 l2 avglen; do
  eval "val=\$$v"
  [ "$val" = K ] || bad="$bad $v"
done
[ -z "$bad" ] && echo CLEAN || echo "LEAKED:$bad"'
out=$(env CANARY_NIGHT_MULT=100 CANARY_STATE_FILE="$TMP/ns" bash -c "$probe" 2>&1)
assert_eq "no-namespace-pollution" "$out" "CLEAN"

# --- 13. bash DEBUG-trap wiring records exactly one command per prompt --------
out=$(run KEEP_TRAP -- 'PROMPT_COMMAND=; _canary_precmd >/dev/null; :; :; :; echo $CANARY_PROMPT_COUNT')
assert_eq "debug-trap-once" "$out" "1"

# --- 14. bash PROMPT_COMMAND is prepended, never clobbered -------------------
out=$(env CANARY_NIGHT_MULT=100 CANARY_STATE_FILE="$TMP/pc" \
      bash -c 'PROMPT_COMMAND="echo MINE"; . "'"$SCRIPT"'"; trap - DEBUG; echo "$PROMPT_COMMAND"')
assert_has "prompt-command-kept" "$out" "echo MINE"
assert_has "prompt-command-ours" "$out" "_canary_precmd"

if [ "$fails" -eq 0 ]; then echo "ok — all canary.sh checks passed"; else echo "$fails check(s) failed"; exit 1; fi
