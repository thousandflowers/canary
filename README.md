# canary
a tiny pixel-art bird that lives in your shell prompt and slowly wilts the
longer you grind. **for fun** — a nudge to step away, not a science.
no color. no internet. no dependency. just UTF-8 block art + your shell.

## bird
```
 ▗███▖        ▗███▖♪       ▗███▖        ▗▓▓▓▖        ▗░░░▖
▐ O ▌>       ▐ ^ ▌>       ▐ - ▌>       ▐ ~ ▌>       ░ x ▌v
 fresh        chirpy       tired        worn         dead
```
in your real prompt it perches just above the line you type:
```
 ▗███▖
▐ O ▌>
❯ git status
```
bird wilts from: time at the shell, how many commands, how long they are.
late night counts extra. dead bird = go rest.

## demo
![canary wilting across a session](assets/demo.gif)
regenerate any time with [vhs](https://github.com/charmbracelet/vhs):
```sh
vhs demo.tape
```

## why

my girlfriend kept telling me i spend too much time at the computer. she
was right, but i needed to see it for myself.

miners used to bring canaries underground — a living warning system, silent
until something was wrong. i liked that image: a creature that absorbs the
environment and shows you its state, without asking you to think about it.

the other reference is [Birdie by Rosendahl](https://us.rosendahl.com/pages/birdie)
— a small brass bird that tilts when CO₂ in the room gets too high. no screen,
no alert, just a physical object changing state. that's what i wanted in the
terminal: something ambient, not another notification.

so: one pixel-art bird, five states, no internet. it just sits there and
slowly wilts while you work.

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
less install.sh canary.sh      # look first
sh install.sh
```
then open a new shell. the bird appears above your prompt.

## tame the bird
```sh
CANARY_DISABLED=1       # bird sleeps (no output)
CANARY_RESET=1          # fresh session, bird young again
CANARY_SHOW_SCORE=1     # show the fatigue number 0–100
# quieter: only show the bird once it actually matters
CANARY_MIN_SCORE=46     # draw only at tired+ (0 = always, default; 71 = worn+)
# idle-aware: coffee/lunch breaks don't age the bird — only active work counts
CANARY_IDLE_THRESHOLD=300  # a gap longer than this (sec) stops the clock (default 300 = 5 min)
# circadian penalty — defaults assume a daytime schedule.
# night owl? tune or switch it off:
CANARY_NIGHT_START=22   # hour the penalty starts (default 22)
CANARY_NIGHT_END=7      # hour it ends            (default 7)
CANARY_NIGHT_MULT=130   # penalty percent: 100 = off, 130 = ×1.3 (default)
```

## the "fatigue" number
```
shell:        minutes/3 + commands/2 + avg_cmd_len/10
Claude Code:  minutes/3 + turns/2 + errors·3 + cadence + reps·2
both          × CANARY_NIGHT_MULT/100 (night)  + multi-day debt   (capped at 100)
0–20 fresh   21–45 chirpy   46–70 tired   71–90 worn   91–100 dead
```
*minutes* = **active** time only: gaps longer than `CANARY_IDLE_THRESHOLD`
(default 5 min) are treated as breaks and don't count. leave the terminal
open all afternoon — the bird only ages while you actually work.

**honest caveat:** this is a crude *activity* proxy, not real cognitive load.
a deep flow session and a frustrating debug both look identical to it. treat
the bird as a playful timer, not a doctor.

## Claude Code statusline
canary also perches next to caveman's `[CAVEMAN]` badge in Claude Code's status line:
```
[CAVEMAN] ▗███▖ tired · 58m · 41t
          ▐ - ▌>
```
Here the bird watches your **coding session**, not your shell. Claude Code pipes its
session JSON to `canary-statusline.sh` on every refresh; the script reads
`cost.total_duration_ms` for minutes and walks the session transcript for richer
signals than the shell bird can see:

- **turns** — your actual prompts (tool-call chatter filtered out), the `41t` above
- **errors** — failed tool calls, a decent proxy for frustration
- **cadence** — turns-per-hour above normal = a frantic, stuck pace
- **reps** — the same command fired back-to-back = spinning your wheels

It's the same five birds and bands; just fed better data. Outside Claude Code (or
before a transcript exists) it falls back to `~/.canary/canary-state`, so the
shell-prompt bird and the statusline tell the same story.

```sh
CANARY_ERR_WEIGHT=3      # points per failed tool call (default 3)
CANARY_CADENCE_BASE=30   # turns/hour treated as "normal" (above it adds points)
CANARY_REP_WEIGHT=2      # points per extra back-to-back repeat of a command
CANARY_DEBT_MAX=30       # cap on yesterday's fatigue carried into today
CANARY_DEAD_ABSOLUTE=1   # always show the dead bird at >90 (default: only when
                         # today is worse than your own recent average — so a
                         # nightly grind doesn't make the dead bird wallpaper)
```

**multi-day debt:** a tiny `~/.canary/history` (one `epoch-day peak` line per day,
pruned to 10) means the bird doesn't reset to fresh just because you opened a new
session — yesterday's peak carries over, halving each day, and fades after ~4–5 days
of rest. Two+ consecutive days past the limit add a `✕ N nights past your limit` line.

`install.sh` wires this automatically (needs `jq`): it drops `canary-statusline.sh`
into `~/.canary/` and merges a `statusLine` command into
`$CLAUDE_CONFIG_DIR/settings.json` (default `~/.claude`). If a status line already
exists (e.g. caveman's), canary is **appended** to it, not replacing it — Claude
Code allows only one status line command, and caveman emits no trailing newline so
the bird lands right beside the badge. A backup is saved to `settings.json.canary.bak`.

`sh uninstall.sh` removes only canary's segment, leaving the rest intact.

## uninstall
```sh
sh uninstall.sh
```
bird gone. rc cleaned. `~/.canary` removed.

## shell
zsh, bash, fish. UTF-8 terminal required.

## license
MIT. see LICENSE.
