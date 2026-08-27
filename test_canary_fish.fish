#!/usr/bin/env fish
# Self-check for canary.fish. No framework, just asserts.
#   fish test_canary_fish.fish   # exits non-zero on any failure
#
# canary.fish bails out early unless the shell is interactive, so every probe
# runs in `fish --no-config -i -c`: the interactive guard is satisfied and no
# config.fish of the developer's leaks in.
#
# The point of this suite is PARITY. The fish bird and the sh bird must agree on
# the score, the bands and the state-file format, or the statusline tells one
# story and the prompt another.
#
# Note: everything shared with the helper functions is `set -g` — a fish
# function cannot see a variable that is merely local to the script scope.

set -g HERE (cd (dirname (status filename)); and pwd)
set -g SCRIPT "$HERE/canary.fish"
set -g TMP (mktemp -d)
set -g fails 0

# Paths get interpolated into `fish -c "..."` strings below, so they must be
# escaped: a repo checked out under e.g. "~/Progetti dev/canary" would otherwise
# split on the space and source nothing at all.
set -g SCRIPT_Q (string escape -- $SCRIPT)
set -g TMP_Q (string escape -- $TMP)

function assert_eq --argument-names label got want
    test "$got" = "$want"; and return 0
    echo "FAIL [$label]: expected '$want', got '$got'"
    set -g fails (math $fails + 1)
end

function assert_has --argument-names label got want
    string match -q -- "*$want*" "$got"; and return 0
    echo "FAIL [$label]: expected to contain '$want'"
    echo "  got: $got"
    set -g fails (math $fails + 1)
end

# run <fish code> — fresh interactive fish with canary.fish sourced.
# stdout is joined with newlines so callers can quote it as one string.
function run
    fish --no-config -i -c "set -g CANARY_NIGHT_MULT 100; set -g CANARY_STATE_FILE $TMP_Q/s"(random)"; source $SCRIPT_Q; $argv[1]" 2>/dev/null | string join \n
end

# --- 1. every band renders its own bird --------------------------------------
for row in "0 fresh ▐ O ▌>" "20 fresh ▐ O ▌>" "21 chirpy ▐ ^ ▌>" "45 chirpy ▐ ^ ▌>" \
           "46 tired ▐ - ▌>" "70 tired ▐ - ▌>" "71 worn ▐ ~ ▌>" "90 worn ▐ ~ ▌>" \
           "91 dead ░ x ▌v" "100 dead ░ x ▌v"
    set parts (string split ' ' -- $row)
    set score $parts[1]
    set want_name $parts[2]
    set want_eye (string join ' ' $parts[3..-1])
    set out (run "_canary_render $score show")
    assert_has "band-$score-name" "$out" "[$want_name $score]"
    assert_has "band-$score-eye" "$out" "$want_eye"
end

set out (run "_canary_render 95 show")
assert_has "dead-nag" "$out" 'CANARY_RESET 1'
set out (run "_canary_render 90 show")
if string match -q -- '*CANARY_RESET 1*' "$out"
    echo "FAIL [worn-no-nag]: worn should not nag"
    set -g fails (math $fails + 1)
end

# --- 2. score formula, identical to canary.sh --------------------------------
# 3600s = 60min -> 26 ; 40 prompts -> 20 ; avg len 50 -> 5 ; total 51
set out (run "set -g CANARY_ACTIVE_SECONDS 3600; set -g CANARY_PROMPT_COUNT 40; set -g CANARY_LEN_SUM 2000; _canary_score")
assert_eq "score-formula" "$out" "51"

# the concave time curve must match canary.sh point for point, or the prompt
# bird and the statusline bird disagree about the same session
for row in "0 0" "15 7" "30 14" "60 26" "120 43" "180 55" "300 72" "480 86" "720 97"
    set parts (string split ' ' -- $row)
    set out (run "_canary_time_points $parts[1]")
    assert_eq "time-curve-$parts[1]m" "$out" "$parts[2]"
end

# ...and so must the circadian table
for row in "0 25" "2 50" "4 50" "5 40" "7 15" "9 0" "13 15" "16 5" "20 0" "22 5" "23 15"
    set parts (string split ' ' -- $row)
    set out (run "_canary_circadian_excess $parts[1]")
    assert_eq "circadian-$parts[1]h" "$out" "$parts[2]"
end

# CANARY_NIGHT_MULT=100 disables the time-of-day adjustment at any hour
set out (run "set -g CANARY_ACTIVE_SECONDS 3600; set -g CANARY_NIGHT_MULT 100; _canary_score")
assert_eq "circadian-disabled" "$out" "26"

set out (run "set -g CANARY_ACTIVE_SECONDS 999999; set -g CANARY_PROMPT_COUNT 9999; _canary_score")
assert_eq "score-clamped" "$out" "100"

set out (run "_canary_score")
assert_eq "score-zero" "$out" "0"

# --- 3. mean command length ---------------------------------------------------
set out (run "_canary_avg")
assert_eq "avg-empty" "$out" "0"
set out (run "set -g CANARY_PROMPT_COUNT 3; set -g CANARY_LEN_SUM 60; _canary_avg")
assert_eq "avg-mean" "$out" "20"

# sum and denominator must stay in step, exactly as in canary.sh
# bare (...) inside a double-quoted fish string is literal, so this substitution
# runs in the child shell, not here
set out (run "for i in (seq 25); _canary_record xx; end; echo \$CANARY_LEN_SUM/\$CANARY_PROMPT_COUNT=(_canary_avg)")
assert_eq "avg-running-sum" "$out" "50/25=2"

# an ignored empty command must move neither
set out (run "_canary_record abcd; _canary_record ''; echo \$CANARY_LEN_SUM/\$CANARY_PROMPT_COUNT")
assert_eq "avg-empty-not-summed" "$out" "4/1"

# --- 4. idle gaps don't age the bird -----------------------------------------
set out (run "set -g CANARY_IDLE_THRESHOLD 1; set -g CANARY_LAST_ACTIVE (math (date +%s) - 3600); _canary_record ls; echo \$CANARY_ACTIVE_SECONDS")
assert_eq "idle-gap-ignored" "$out" "0"

# tolerance, not equality: the fixture and _canary_record each call date(1), so
# a second boundary between them makes this 601 — a spurious failure otherwise
set out (run "set -g CANARY_IDLE_THRESHOLD 99999; set -g CANARY_LAST_ACTIVE (math (date +%s) - 600); _canary_record ls; test \$CANARY_ACTIVE_SECONDS -ge 600 -a \$CANARY_ACTIVE_SECONDS -le 602; and echo in-range; or echo \$CANARY_ACTIVE_SECONDS")
assert_eq "active-gap-accrued" "$out" "in-range"

# --- 5. bare Enter is not a prompt -------------------------------------------
set out (run "_canary_record ''; echo \$CANARY_PROMPT_COUNT")
assert_eq "empty-not-counted" "$out" "0"

# --- 6. time of day: a bigger multiplier may only raise the score ------------
set out (run "set -g CANARY_ACTIVE_SECONDS 3600; set -g CANARY_NIGHT_MULT 150; test (_canary_score) -ge 26; and echo ge; or echo LOWER")
assert_eq "circadian-monotone" "$out" "ge"

# --- 7. quiet threshold ------------------------------------------------------
set out (run "set -g CANARY_MIN_SCORE 50; _canary_compute")
assert_eq "min-score-quiet" "$out" ""
set out (run "set -g CANARY_MIN_SCORE 0; _canary_compute")
assert_has "min-score-loud" "$out" '▐ O ▌>'

# --- 8. state file: byte-compatible with what canary-statusline.sh reads -----
set -g S "$TMP/state.explicit"
fish --no-config -i -c "set -g CANARY_STATE_FILE "(string escape -- $S)"; source $SCRIPT_Q; _canary_record hello; _canary_record worldly" >/dev/null 2>&1
set out (cat $S 2>/dev/null | string join '|')
assert_has "state-count" "$out" "prompt_count=2"
assert_has "state-avg" "$out" "avg_prompt_len=6"
assert_has "state-active" "$out" "active_seconds="

# the statusline must consume what the fish bird wrote
set out (env CANARY_NIGHT_MULT=100 CANARY_STATE_FILE=$S CANARY_HISTORY_FILE=$TMP/hist bash "$HERE/canary-statusline.sh" </dev/null | string join \n)
assert_has "state-roundtrip" "$out" "2p"

# --- 9. reset wipes the session ----------------------------------------------
set out (run "_canary_record ls; set -g CANARY_RESET 1; _canary_compute >/dev/null; echo \$CANARY_PROMPT_COUNT/\$CANARY_LEN_SUM/\$CANARY_ACTIVE_SECONDS/"'(set -q CANARY_RESET; and echo set; or echo unset)')
assert_eq "reset" "$out" "0/0/0/unset"

# --- 10. the `canary` command ------------------------------------------------
set out (run "canary")
assert_has "cmd-status" "$out" '▐ O ▌>'
set out (run "canary score")
assert_eq "cmd-score" "$out" "0"
set out (run "canary reset")
assert_has "cmd-reset" "$out" 'canary: reset'
set out (run "canary --help")
assert_has "cmd-help" "$out" 'usage: canary'
set out (run "canary off")
assert_has "cmd-off" "$out" 'canary: off'
set out (run "canary bogus >/dev/null 2>&1; echo exit=\$status")
assert_eq "cmd-unknown-exit" "$out" "exit=1"

# --- 11. re-sourcing is a no-op (load guard) ---------------------------------
set out (run "_canary_record ls; source $SCRIPT_Q; echo \$CANARY_PROMPT_COUNT")
assert_eq "load-guard" "$out" "1"

# --- 12. the bird prints two rows, not one -----------------------------------
set out (run "_canary_render 5 | count")
assert_eq "bird-two-rows" "$out" "2"

# --- 13. fish_prompt is wrapped, not destroyed -------------------------------
set out (fish --no-config -i -c "function fish_prompt; echo MINE; end; source $SCRIPT_Q; set -g CANARY_MIN_SCORE 99; fish_prompt" 2>/dev/null | string join \n)
assert_has "prompt-wrapped" "$out" "MINE"

rm -rf $TMP
if test $fails -eq 0
    echo "ok — all canary.fish checks passed"
else
    echo "$fails check(s) failed"
    exit 1
end
