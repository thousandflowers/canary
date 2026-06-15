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
set -q CANARY_LENS;        or set -g CANARY_LENS

# --- tunables (night penalty configurable; CANARY_NIGHT_MULT=100 disables) ---
set -g _CANARY_LEN_WINDOW 20
set -q CANARY_NIGHT_START; or set -g CANARY_NIGHT_START 22
set -q CANARY_NIGHT_END;   or set -g CANARY_NIGHT_END 7
set -q CANARY_NIGHT_MULT;  or set -g CANARY_NIGHT_MULT 130

# --- record each command (fish fires fish_preexec with the command line) -----
function _canary_record --on-event fish_preexec
    set -l cmd $argv[1]
    test -n "$cmd"; or return
    set -g CANARY_PROMPT_COUNT (math $CANARY_PROMPT_COUNT + 1)
    set -ga CANARY_LENS (string length -- "$cmd")
    set -l n (count $CANARY_LENS)
    if test $n -gt $_CANARY_LEN_WINDOW
        set -e CANARY_LENS[1..(math $n - $_CANARY_LEN_WINDOW)]
    end
end

# --- rolling average ---------------------------------------------------------
function _canary_avg
    set -l n (count $CANARY_LENS)
    if test $n -eq 0
        echo 0
        return
    end
    set -l sum 0
    for x in $CANARY_LENS
        set sum (math $sum + $x)
    end
    math "floor($sum / $n)"
end

# --- map score -> art, set CANARY_BIRD, print -------------------------------
function _canary_render
    set -l score $argv[1]
    set -l force $argv[2]   # non-empty -> always show the score line
    set -l name; set -l l1; set -l l2
    if test $score -le 20
        set name fresh;  set l1 ' ▗███▖';  set l2 '▐ ◉ ▌>'
    else if test $score -le 45
        set name chirpy; set l1 ' ▗███▖~'; set l2 '▐ ^ ▌>'
    else if test $score -le 70
        set name tired;  set l1 ' ▗███▖';  set l2 '▐ - ▌>'
    else if test $score -le 90
        set name worn;   set l1 ' ▗▓▓▓▖';  set l2 '▐ ~ ▌>'
    else
        set name dead;   set l1 ' ▗░░░▖';  set l2 '░ x ▌v'
    end

    set -g CANARY_BIRD "$l1\n$l2"

    if set -q CANARY_SHOW_SCORE; or test -n "$force"
        printf '%s\n%s  [%s %s]\n' $l1 $l2 $name $score
    else
        printf '%s\n%s\n' $l1 $l2
    end

    test "$name" = dead; and printf '%s\n' '  tweet… you look fried. reset with  set -x CANARY_RESET 1'
end

# --- compute the 0-100 fatigue score from current session state -------------
function _canary_score
    set -l now (date +%s)
    set -l min (math "floor(($now - $CANARY_START_TIME) / 60)")
    set -l avglen (_canary_avg)
    set -l s (math "floor($min / 3 + $CANARY_PROMPT_COUNT / 2 + $avglen / 10)")

    set -l hour (date +%H | sed 's/^0*//')
    test -z "$hour"; and set hour 0
    if test $hour -ge $CANARY_NIGHT_START -o $hour -lt $CANARY_NIGHT_END
        set s (math "floor($s * $CANARY_NIGHT_MULT / 100)")
    end

    test $s -gt 100; and set s 100
    echo $s
end

# --- per-prompt: honor reset, recompute, draw -------------------------------
function _canary_compute
    set -q CANARY_DISABLED; and return

    if set -q CANARY_RESET
        set -g CANARY_START_TIME (date +%s)
        set -g CANARY_PROMPT_COUNT 0
        set -g CANARY_LENS
        set -e CANARY_RESET
    end

    _canary_render (_canary_score)
end

# --- `canary` command: on-demand status / control ---------------------------
function canary
    set -l cmd $argv[1]
    test -z "$cmd"; and set cmd status
    switch $cmd
        case status --status
            _canary_render (_canary_score) show
        case score --score
            _canary_score
        case reset --reset
            set -g CANARY_START_TIME (date +%s)
            set -g CANARY_PROMPT_COUNT 0
            set -g CANARY_LENS
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
