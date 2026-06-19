# canary

a tiny pixel-art bird that lives in **Claude Code's status line** and slowly
wilts the longer and harder your session runs. **for fun** — a nudge to step
away, not a science.

no color. no internet. no dependency. just UTF-8 block art + your session.

## bird

```
 ▗███▖        ▗███▖♪       ▗███▖        ▗▓▓▓▖        ▗░░░▖
▐ O ▌>       ▐ ^ ▌>       ▐ - ▌>       ▐ ~ ▌>       ░ x ▌v
 fresh        chirpy       tired        worn         dead
```

in Claude Code it perches in the status line, beside whatever else you run
there (e.g. caveman's badge):

```
[CAVEMAN] ▗███▖ tired · 58m · 41t
          ▐ - ▌>
```

bird wilts from: time in the session + how many turns you've taken. late night
counts extra. dead bird = go rest.

## install

**Homebrew** (cleanest):

```sh
brew install thousandflowers/tap/canary
```

or one line:

```sh
curl -fsSL https://raw.githubusercontent.com/thousandflowers/canary/main/install.sh | sh
```

**prefer to read before you run?** (good instinct — `curl | sh` runs code
sight-unseen.) clone, inspect, then install locally:

```sh
git clone https://github.com/thousandflowers/canary
cd canary
less install.sh canary-statusline.sh      # look first
sh install.sh
```

then restart Claude Code (or reload the window). the bird appears in the status line.

`install.sh` drops `canary-statusline.sh` into `~/.canary/` and merges a
`statusLine` command into `$CLAUDE_CONFIG_DIR/settings.json` (default
`~/.claude`). If a status line already exists (e.g. caveman's), canary is
**appended** to it, not replacing it — Claude Code allows only one status line
command. A backup is saved to `settings.json.canary.bak`. Needs `jq`; without
it, the installer prints the line to add by hand.

## how it works

Claude Code pipes a status JSON to the command on every refresh.
`canary-statusline.sh` reads it, scores fatigue, and prints the two-line bird.
It writes nothing and calls no network — pure shell + `grep`.

## tame the bird

Export these into Claude Code's environment (e.g. via `settings.json` `env`):

```sh
CANARY_DISABLED=1       # bird sleeps (no output)
CANARY_SHOW_SCORE=1     # show the fatigue number 0–100

# quieter: only show the bird once it actually matters
CANARY_MIN_SCORE=46     # draw only at tired+ (0 = always, default; 71 = worn+)

# how hard a failed tool call (a debug slog) wilts the bird
CANARY_ERR_WEIGHT=3     # score points per errored tool call (default 3; 0 = ignore errors)
CANARY_CADENCE_BASE=30  # turns/hour above which the pace counts as frantic (default 30)
CANARY_REP_WEIGHT=2     # score per extra repeat in the longest same-command run (default 2)

# multi-day fatigue: a hard week carries over, rest pays it down
CANARY_DEBT_MAX=30      # cap on carried-over fatigue from recent days (default 30; 0 = off)
CANARY_HISTORY_FILE=…   # where daily peaks live (default ~/.canary/history)

# circadian penalty — defaults assume a daytime schedule.
# night owl? tune or switch it off:
CANARY_NIGHT_START=22   # hour the penalty starts (default 22)
CANARY_NIGHT_END=7      # hour it ends            (default 7)
CANARY_NIGHT_MULT=130   # penalty percent: 100 = off, 130 = ×1.3 (default)
```

## the "fatigue" number

```
raw   = minutes/3 + turns/2 + errors×3 + cadence + reps×2   (today, capped 100)
        × CANARY_NIGHT_MULT/100   (at night)
score = raw + multi-day debt      (capped at 100)

0–20 fresh   21–45 chirpy   46–70 tired   71–90 worn   91–100 dead
```

*minutes* = session wall-clock (`cost.total_duration_ms`).
*turns* = your messages in the session transcript.
*errors* = tool calls that errored (`is_error:true`) — frustration.
*cadence* = frantic pace: turns/hour above `CANARY_CADENCE_BASE` = stuck, not flow.
*reps* = longest run of the **same command back-to-back** — ran it 5× in a row?
you're looping. (`uniq` counts consecutive only, so interleaved tool noise is ignored.)
*multi-day debt* = a hard week carries over. canary keeps each day's peak in
`~/.canary/history` and adds a decaying share of recent days — halved per day,
so a few days of rest pay it down. you start Monday tired after a brutal week.

**honest caveat:** still a proxy, not a doctor — but it now tells a smooth run
from a slog, a frantic loop from steady flow, and a rested you from a fried one.

## uninstall

```sh
sh uninstall.sh
```

bird gone. status line cleaned. `~/.canary` removed.

## requires

Claude Code, a UTF-8 terminal, and `jq` (for the installer only).

## license

MIT. see LICENSE.
