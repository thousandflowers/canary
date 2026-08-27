# canary.fish — pixel-art fatigue bird for the fish shell.
# Zero deps, no ANSI color, no API calls. Wired by install.sh into config.fish.
#
# Knobs:  CANARY_DISABLED=1  CANARY_RESET=1  CANARY_SHOW_SCORE=1

status is-interactive; or return 0
set -q CANARY_DISABLED; and return 0
set -q _CANARY_LOADED; and return 0
set -g _CANARY_LOADED 1

# --- session state -----------------------------------------------------------
set -q CANARY_START_TIME;  or set -g CANARY_START_TIME (date +%s)
set -q CANARY_PROMPT_COUNT; or set -g CANARY_PROMPT_COUNT 0
set -q CANARY_LEN_SUM;     or set -g CANARY_LEN_SUM 0
set -q CANARY_ACTIVE_SECONDS; or set -g CANARY_ACTIVE_SECONDS 0
set -q CANARY_LAST_ACTIVE;    or set -g CANARY_LAST_ACTIVE $CANARY_START_TIME
set -q CANARY_STATE_FILE;     or set -g CANARY_STATE_FILE $HOME/.canary/canary-state
mkdir -p (dirname $CANARY_STATE_FILE) 2>/dev/null

# --- tunables (CANARY_NIGHT_MULT is the multiplier at the deepest circadian
# trough, 02:00-04:00; 100 switches the time-of-day adjustment off) -----------
set -q CANARY_NIGHT_MULT;  or set -g CANARY_NIGHT_MULT 150
set -q CANARY_IDLE_THRESHOLD; or set -g CANARY_IDLE_THRESHOLD 300
set -q CANARY_MIN_SCORE;      or set -g CANARY_MIN_SCORE 0

# --- record each command (fish fires fish_preexec with the command line) -----
function _canary_record --on-event fish_preexec
    set -l cmd $argv[1]
    test -n "$cmd"; or return

    # accrue active time, ignoring idle gaps (coffee breaks don't tire the bird)
    set -l now (date +%s)
    set -l gap (math $now - $CANARY_LAST_ACTIVE)
    test $gap -le $CANARY_IDLE_THRESHOLD; and set -g CANARY_ACTIVE_SECONDS (math $CANARY_ACTIVE_SECONDS + $gap)
    set -g CANARY_LAST_ACTIVE $now

    set -g CANARY_PROMPT_COUNT (math $CANARY_PROMPT_COUNT + 1)
    set -g CANARY_LEN_SUM (math $CANARY_LEN_SUM + (string length -- "$cmd"))

    _canary_write_state
end

# --- mean command length, over the whole session -----------------------------
# A running sum over the same commands PROMPT_COUNT counts — see canary.sh for
# why the last-20 window went away.
function _canary_avg
    test $CANARY_PROMPT_COUNT -gt 0; or begin
        echo 0
        return
    end
    math "floor($CANARY_LEN_SUM / $CANARY_PROMPT_COUNT)"
end

# --- persist session state for the Claude Code statusline --------------------
function _canary_write_state
    test -n "$CANARY_STATE_FILE"; or return
    printf 'prompt_count=%s\navg_prompt_len=%s\nactive_seconds=%s\n' \
        $CANARY_PROMPT_COUNT (_canary_avg) $CANARY_ACTIVE_SECONDS \
        >$CANARY_STATE_FILE 2>/dev/null
end

# --- map score -> art, print -------------------------------------------------
function _canary_render
    set -l score $argv[1]
    set -l force $argv[2]   # non-empty -> always show the score line
    set -l name; set -l l1; set -l l2
    if test $score -le 20
        set name fresh;  set l1 ' ▗███▖';   set l2 '▐ O ▌>'
    else if test $score -le 45
        set name chirpy; set l1 ' ▗███▖ ♪'; set l2 '▐ ^ ▌>'
    else if test $score -le 70
        set name tired;  set l1 ' ▗███▖';  set l2 '▐ - ▌>'
    else if test $score -le 90
        set name worn;   set l1 ' ▗▓▓▓▖';  set l2 '▐ ~ ▌>'
    else
        set name dead;   set l1 ' ▗░░░▖';  set l2 '░ x ▌v'
    end

    if set -q CANARY_SHOW_SCORE; or test -n "$force"
        printf '%s\n%s  [%s %s]\n' $l1 $l2 $name $score
    else
        printf '%s\n%s\n' $l1 $l2
    end

    test "$name" = dead; and printf '%s\n' '  tweet… you look fried. reset with  set -x CANARY_RESET 1'
end

# --- active minutes -> points ------------------------------------------------
# Concave, not linear — see canary.sh for the reasoning and the reference curve.
# Must stay identical to canary.sh or the prompt bird and the statusline bird
# tell different stories about the same session.
function _canary_time_points
    math "floor($argv[1] * 130 / ($argv[1] + 240))"
end

# --- time-of-day: percentage points added, at full amplitude -----------------
# Nadir 02:00-06:00, deepest 02:00-04:00; post-lunch dip 13:00-16:00;
# 17:00-21:00 is the evening wake maintenance zone, so no penalty there.
function _canary_circadian_excess
    switch $argv[1]
        case 2 3 4
            echo 50
        case 5 6
            echo 40
        case 0 1
            echo 25
        case 7 13 14 15 23
            echo 15
        case 16 22
            echo 5
        case '*'
            echo 0
    end
end

# --- compute the 0-100 fatigue score from current session state -------------
function _canary_score
    set -l min (math "floor($CANARY_ACTIVE_SECONDS / 60)")
    set -l avglen (_canary_avg)
    set -l s (math "floor("(_canary_time_points $min)" + $CANARY_PROMPT_COUNT / 2 + $avglen / 10)")

    set -l hour (date +%H | sed 's/^0*//')
    test -z "$hour"; and set hour 0
    # CANARY_NIGHT_MULT scales the whole curve, so 100 disables it.
    set -l excess (math "floor("(_canary_circadian_excess $hour)" * ($CANARY_NIGHT_MULT - 100) / 50)")
    test $excess -gt 0; and set s (math "floor($s * (100 + $excess) / 100)")

    test $s -gt 100; and set s 100
    echo $s
end

# --- per-prompt: honor reset, recompute, draw -------------------------------
function _canary_compute
    set -q CANARY_DISABLED; and return

    if set -q CANARY_RESET
        set -g CANARY_START_TIME (date +%s)
        set -g CANARY_PROMPT_COUNT 0
        set -g CANARY_LEN_SUM 0
        set -g CANARY_ACTIVE_SECONDS 0
        set -g CANARY_LAST_ACTIVE $CANARY_START_TIME
        set -e CANARY_RESET
        _canary_write_state
    end

    set -l score (_canary_score)
    test $score -lt $CANARY_MIN_SCORE; and return   # stay quiet below threshold
    _canary_render $score
end

# --- `canary` command: on-demand status / control ---------------------------
function canary
    set -l cmd $argv[1]
    test -z "$cmd"; and set cmd status
    switch $cmd
        case status
            _canary_render (_canary_score) show
        case score
            _canary_score
        case reset
            set -g CANARY_START_TIME (date +%s)
            set -g CANARY_PROMPT_COUNT 0
            set -g CANARY_LEN_SUM 0
            set -g CANARY_ACTIVE_SECONDS 0
            set -g CANARY_LAST_ACTIVE $CANARY_START_TIME
            _canary_write_state
            echo "canary: reset"
            _canary_render (_canary_score) show
        case off
            set -gx CANARY_DISABLED 1
            echo "canary: off (set -e CANARY_DISABLED to re-enable)"
        case on
            set -e CANARY_DISABLED
            echo "canary: on"
        case -h --help help
            echo "usage: canary [status|score|reset|on|off]"
        case '*'
            echo "canary: unknown command: $cmd" >&2
            return 1
    end
end

# --- draw above the prompt by wrapping fish_prompt once ----------------------
if functions -q fish_prompt
    functions -c fish_prompt _canary_user_prompt
end

function fish_prompt
    _canary_compute
    if functions -q _canary_user_prompt
        _canary_user_prompt
    else
        printf '%s> ' (prompt_pwd)
    end
end
