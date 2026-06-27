#!/usr/bin/env bash
# Self-check for canary-statusline.sh — no framework, just asserts.
#   bash test_canary_statusline.sh   # exits non-zero on any failure
#
# Circadian penalty and cadence are pinned off (NIGHT_MULT=100, CADENCE_BASE
# huge) so scores are deterministic regardless of wall-clock time of day.

set -u
HERE=$(cd "$(dirname "$0")" && pwd)
SCRIPT="$HERE/canary-statusline.sh"
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

fails=0
assert_has()  { case "$2" in *"$3"*) ;; *) echo "FAIL [$1]: expected to contain '$3'"; echo "  got: $(printf '%s' "$2" | tr '\n' '|')"; fails=$((fails+1));; esac; }
assert_empty(){ [ -z "$2" ] && return; echo "FAIL [$1]: expected empty, got: $2"; fails=$((fails+1)); }

# build a transcript: $1=file $2=#human $3=#tool_results(ok) $4=#errors
mk() {
  : > "$1"; i=0
  while [ "$i" -lt "$2" ]; do echo '{"type":"user","message":{"role":"user","content":"hi"}}' >> "$1"; i=$((i+1)); done
  i=0; while [ "$i" -lt "$3" ]; do echo '{"type":"user","message":{"content":[{"type":"tool_result"}]}}' >> "$1"; i=$((i+1)); done
  i=0; while [ "$i" -lt "$4" ]; do echo '{"type":"user","message":{"content":[{"type":"tool_result","is_error":true}]}}' >> "$1"; i=$((i+1)); done
}
ccjson() { printf '{"transcript_path":"%s","cost":{"total_duration_ms":%s}}' "$1" "$2"; }

run() { # stdin, extra env... → output. fresh history per run unless caller pins it.
  local in=$1; shift
  printf '%s' "$in" | env CANARY_NIGHT_MULT=100 CANARY_CADENCE_BASE=999999 \
    CANARY_HISTORY_FILE="$TMP/h.$RANDOM" "$@" bash "$SCRIPT"
}

# 1) fresh: 2 turns, 3 min → raw = 3/3 + 2/2 = 2
mk "$TMP/t1" 2 0 0
out=$(run "$(ccjson "$TMP/t1" 180000)" CANARY_SHOW_SCORE=1)
assert_has "fresh-band" "$out" "fresh"
assert_has "fresh-eye"  "$out" "▐ O ▌>"
assert_has "fresh-turns" "$out" "2t"

# 2) tired: 60 min, 40 turns, 5 errors → 20 + 20 + 15 = 55
mk "$TMP/t2" 40 0 5
out=$(run "$(ccjson "$TMP/t2" 3600000)" CANARY_SHOW_SCORE=1)
assert_has "tired-band"  "$out" "tired"
assert_has "tired-eye"   "$out" "▐ - ▌>"
assert_has "tired-score" "$out" "· 55"
assert_has "tired-err"   "$out" "5e"

# 3) dead: huge session, raw caps at 100, no history so raw>personal(0) → stays dead
mk "$TMP/t3" 300 0 0
out=$(run "$(ccjson "$TMP/t3" 7200000)")
assert_has "dead-band" "$out" "dead"
assert_has "dead-eye"  "$out" "▐ x ▌v"

# 4) reps: same command 6x in a row → reps=5 → +10. 5 min, 6 turns: 1+3+10 = 14 (fresh)
:> "$TMP/t4"; i=0; while [ "$i" -lt 6 ]; do echo '{"type":"assistant","message":{"content":[{"type":"tool_use","input":{"command":"ls -la"}}]}}' >> "$TMP/t4"; echo '{"type":"user","message":{"role":"user","content":"go"}}' >> "$TMP/t4"; i=$((i+1)); done
out=$(run "$(ccjson "$TMP/t4" 300000)" CANARY_SHOW_SCORE=1)
assert_has "reps-band" "$out" "fresh"      # 1 + 3 + 10 = 14 ≤ 20

# 5) shell fallback: no transcript_path → reads canary-state
printf 'timestamp_start=0\nactive_seconds=600\nprompt_count=10\navg_prompt_len=50\n' > "$TMP/state"
out=$(run "" CANARY_STATE_FILE="$TMP/state")
assert_has "shell-band"  "$out" "fresh"    # 10/3 + 10/2 + 50/10 = 3+5+5 = 13
assert_has "shell-label" "$out" "10p"

# 6) disabled → nothing
out=$(run "$(ccjson "$TMP/t1" 180000)" CANARY_DISABLED=1)
assert_empty "disabled" "$out"

# 7) min-score quiet threshold suppresses a low score
out=$(run "$(ccjson "$TMP/t1" 180000)" CANARY_MIN_SCORE=50)
assert_empty "min-score" "$out"

# 8) anti-habituation: old high peak (age 10) → personal≈95, ~0 debt. today raw=92
#    (60min→20 + 144turns/2=72) ≤ personal → dead demotes to worn.
H="$TMP/anti"; today=$(( $(date +%s) / 86400 )); echo "$((today-10)) 95" > "$H"
mk "$TMP/t8" 144 0 0
out=$(printf '%s' "$(ccjson "$TMP/t8" 3600000)" | env CANARY_NIGHT_MULT=100 \
  CANARY_CADENCE_BASE=999999 CANARY_HISTORY_FILE="$H" bash "$SCRIPT")
assert_has "anti-demote" "$out" "worn"
assert_has "anti-eye"    "$out" "▐ ~ ▌>"

# 9) escalation: two consecutive prior nights ≥90 → line prints (face decoupled)
H2="$TMP/esc"; printf '%s 95\n%s 95\n' "$((today-1))" "$((today-2))" > "$H2"
mk "$TMP/t9" 2 0 0
out=$(printf '%s' "$(ccjson "$TMP/t9" 180000)" | env CANARY_NIGHT_MULT=100 \
  CANARY_CADENCE_BASE=999999 CANARY_HISTORY_FILE="$H2" bash "$SCRIPT")
assert_has "escalation" "$out" "nights past your limit"

# 10) missing transcript path → no turns, no crash, time-only score
out=$(run "$(ccjson "/nonexistent/$RANDOM" 600000)")
assert_has "missing-transcript" "$out" "fresh"   # 10min/3 only = 3

if [ "$fails" -eq 0 ]; then echo "ok — all canary-statusline checks passed"; else echo "$fails check(s) failed"; exit 1; fi
