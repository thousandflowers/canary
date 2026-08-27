# canary

[![CI](https://github.com/thousandflowers/canary/actions/workflows/ci.yml/badge.svg)](https://github.com/thousandflowers/canary/actions/workflows/ci.yml)

> a toy, on purpose. it is tested (five suites, shellcheck, ubuntu + macOS + fish
> on every push) and the curve is grounded in published fatigue research, but it
> reads keystrokes, not you. treat it as a nudge, never as a measurement.

a tiny pixel-art bird that lives in your shell prompt and slowly wilts the
longer you grind. **for fun** - a nudge to step away, not a science.
no color. no internet. no runtime dependency. one small binary, UTF-8 block
art, and your shell.

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

miners used to bring canaries underground - a living warning system, silent
until something was wrong. i liked that image: a creature that absorbs the
environment and shows you its state, without asking you to think about it.

the other reference is [Birdie by Rosendahl](https://us.rosendahl.com/pages/birdie) -
a small brass bird that tilts when CO₂ in the room gets too high. no screen,
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
**prefer to read before you run?** (good instinct - `curl | sh` runs code
sight-unseen.) clone, inspect, then install locally:
```sh
git clone https://github.com/thousandflowers/canary
cd canary
less install.sh              # look first
sh install.sh                # builds from source if you have Go
```
**already have Go?**
```sh
go install github.com/thousandflowers/canary/cmd/canary@latest
```

then wire your shell — the installer does this for you, and this is what it
adds:
```sh
eval "$(canary init zsh)"          # ~/.zshrc
eval "$(canary init bash)"         # ~/.bashrc
canary init fish | source          # ~/.config/fish/config.fish
canary settings install            # Claude Code's status line
```
open a new shell. the bird appears above your prompt.

<details>
<summary>upgrading from 0.x</summary>

0.x was three shell scripts installed into `~/.canary`. v1.0 is one binary and
nothing else: run `install.sh` again (it replaces the rc line) or, if you want
to be thorough, `sh uninstall.sh` first — it recognises the old source line and
takes it back.

Two things changed on purpose:

- **`canary on` / `canary off` are gone.** A binary cannot change the
  environment of the shell that ran it. `CANARY_DISABLED=1` still works, and
  still silences both birds.
- **the bird is now per-person, not per-terminal.** Three windows used to mean
  three independent birds, while the file the status line read was overwritten
  by whichever of them ran a command last. One state file, shared. Idle gaps
  still do not count, so a terminal you are not typing in ages nothing.

</details>

## tame the bird
```sh
CANARY_DISABLED=1       # bird sleeps (no output)
CANARY_RESET=1          # fresh session, bird young again
CANARY_SHOW_SCORE=1     # show the fatigue number 0–100
# quieter: only show the bird once it actually matters
CANARY_MIN_SCORE=71     # draw only at worn+ — the level worth acting on
                        # (0 = always, the default; 46 = tired+, chattier)
# idle-aware: coffee/lunch breaks don't age the bird — only active work counts
CANARY_IDLE_THRESHOLD=300  # a gap longer than this (sec) stops the clock (default 300 = 5 min)
# circadian penalty — defaults assume a daytime schedule.
# night owl? tune or switch it off:
CANARY_NIGHT_MULT=150   # multiplier at the bottom of the circadian trough
                        # (02:00–04:00). scales the whole curve, so 100 = off.
                        # replaces CANARY_NIGHT_START/END, which are gone — the
                        # penalty is a curve now, not a window with two edges.
```

## the "fatigue" number
```
shell:        time(minutes) + commands/2 + avg_cmd_len/10
Claude Code:  time(minutes) + turns/2    + errors·3 + reps·2
both          × the time-of-day curve  + multi-day debt   (capped at 100)
0–20 fresh   21–45 chirpy   46–70 tired   71–90 worn   91–100 dead
```
*minutes* = **active** time only: gaps longer than `CANARY_IDLE_THRESHOLD`
(default 5 min) are treated as breaks and don't count. leave the terminal
open all afternoon - the bird only ages while you actually work.

**honest caveat:** this is an *activity* proxy, not real cognitive load. a deep
flow session and a frustrating debug can still look alike to it, and the inputs
are keystrokes, not a psychomotor vigilance task. treat the bird as a playful
timer, not a doctor. what follows is why the *shape* of the curve is what it is.

### why these numbers

**the five birds are the five labelled points of a real scale.** the Karolinska
Sleepiness Scale runs 1–9 but is verbally anchored at only five: *extremely
alert* (1), *alert* (3), *neither alert nor sleepy* (5), *sleepy, but no
difficulty remaining awake* (7), *extremely sleepy, fighting sleep* (9). the
birds map straight onto them:

| bird | KSS | means |
|---|---|---|
| fresh | 1 | extremely alert |
| chirpy | 3 | alert |
| tired | 5 | neither alert nor sleepy — the neutral midpoint, **not** impaired |
| worn | **7** | sleepy. **fatigue-risk protocols call for a break here** |
| dead | 9 | fighting sleep |

that's why `CANARY_MIN_SCORE=71` (worn) is the quiet threshold worth setting,
not 46. **tired is not a problem. worn is.**

**time is a curve, not a line.** the vigilance-decrement literature is
consistent that the drop is front-loaded — roughly half of it inside the first
15 minutes, reaction times climbing reliably past 30, costs steepening after 60
— and then it flattens instead of continuing straight up. so:

```
15m→7   30m→14   1h→26   2h→43   3h→55   5h→72   8h→86   12h→97
```

five hours of genuinely active work lands on **worn**, the KSS-7 line. the old
formula was linear `minutes/3`: it under-read the first hour (60 solid minutes
scored 20, still "chirpy") and pinned everything past 5h at 100, which turned
the dead bird into wallpaper.

**the night penalty follows the actual circadian trough.** the nadir is
02:00–06:00 and deepest 02:00–04:00; attention bottoms out 04:00–07:00; there
is a genuine post-lunch dip 13:00–16:00 that is circadian, not caused by
lunch; and 17:00–21:00 is the evening *wake maintenance zone*, where alertness
is high. the old rule — a flat ×1.3 from 22:00 to 07:00 — penalised you hardest
while you were at your sharpest and treated 23:00 exactly like 03:00.

**errors and reps are the best-evidenced signals canary has.** mental fatigue
reliably increases error rates *and* perseveration — continuing to repeat an
action that is not working — which is exactly what `reps` counts. that makes
the Claude Code bird the better instrument: the shell bird can only see time
and typing, and `avg_cmd_len` in particular has no evidence behind it at all.

**why multi-day debt exists.** chronic sleep restriction produces cumulative
deficits comparable to two or three days of total sleep deprivation, and it
does so *"without full awareness of the affected individuals."* you cannot
self-assess your way out of this, which is the whole argument for an ambient
bird instead of asking yourself how you feel.

that finding also fixes the anti-habituation rule. canary calms a dead bird to
worn when today is no worse than your own recent average, because a bird that
is dead every day carries no information — **but not during a streak.** two or
more consecutive days past your limit is precisely the case where you can't
feel it, so the bird stays dead and the `✕ N nights` line keeps counting.

**deliberately not implemented:** 90-minute "ultradian work cycles". Kleitman's
basic rest-activity cycle is solid in *sleep*; in waking it is weakly supported,
reported cycles range 70–120 minutes with large individual variation, and
essentially every source advocating 90-minute work blocks is a productivity
blog rather than peer review. hard-coding it would be inventing precision.

<details>
<summary>sources</summary>

- Åkerstedt & Gillberg, *Subjective and objective sleepiness in the active
  individual* — the KSS; validated against reaction-time lapses, EEG
  alpha/theta and slow eye movements.
- Åkerstedt & Folkard, *The three-process model of alertness and its extension
  to performance, sleep latency, and sleep length*, SLEEP.
- van der Linden, Frese & Meijman, *Mental fatigue and the control of cognitive
  processes: effects on perseveration and planning*, Acta Psychologica (2003).
- Van Dongen, Maislin, Mullington & Dinges, *The cumulative cost of additional
  wakefulness*, SLEEP 26(2):117–126 (2003).
- Albulescu et al., *"Give me a break!" A systematic review and meta-analysis
  on the efficacy of micro-breaks*, PLOS ONE (2022).
- Valdez, *Circadian rhythms in attention*, Yale J Biol Med (2019).

</details>

## Claude Code statusline
canary also perches next to caveman's `[CAVEMAN]` badge in Claude Code's status line:
```
[CAVEMAN] ▗███▖ tired · 58m · 41t
          ▐ - ▌>
```
Here the bird watches your **coding session**, not your shell. Claude Code pipes its
session JSON to `canary statusline` on every refresh; it reads
`cost.total_duration_ms` for minutes and walks the session transcript for richer
signals than the shell bird can see:

- **turns** - your actual prompts (tool-call chatter filtered out), the `41t` above
- **errors** - failed tool calls, a decent proxy for frustration
- **reps** - the same command fired back-to-back = spinning your wheels (capped, so a polling loop won't bury the bird)

It's the same five birds and bands; just fed better data. Outside Claude Code (or
before a transcript exists) it falls back to `~/.canary/canary-state`, so the
shell-prompt bird and the statusline tell the same story.

```sh
CANARY_ERR_WEIGHT=3      # points per failed tool call (default 3)
CANARY_REP_WEIGHT=2      # points per extra back-to-back repeat of a command (capped at 5)
CANARY_DEBT_MAX=30       # cap on yesterday's fatigue carried into today
CANARY_DEAD_ABSOLUTE=1   # always show the dead bird at >90 (default: only when
                         # today is worse than your own recent average — so a
                         # nightly grind doesn't make the dead bird wallpaper)
```

**multi-day debt:** a tiny `~/.canary/history` (one `epoch-day peak` line per day,
pruned to 10) means the bird doesn't reset to fresh just because you opened a new
session - yesterday's peak carries over, halving each day, and fades after ~4–5 days
of rest. Two+ consecutive days past the limit add a `✕ N nights past your limit` line.

`canary settings install` wires this — `install.sh` runs it for you. It merges a
`statusLine` command into `$CLAUDE_CONFIG_DIR/settings.json` (default `~/.claude`).
If a status line already exists (e.g. caveman's), canary is **appended** to it, not
replacing it - Claude Code allows only one status line command, and caveman emits no
trailing newline so the bird lands right beside the badge. A backup is saved to
`settings.json.canary.bak`, a symlinked `settings.json` is written *through* rather
than replaced, and a file with comments in it is left alone with a message instead
of being silently reformatted.

`canary settings remove` takes back only canary's segment, leaving the rest intact.
`sh uninstall.sh` does that and the rc lines and `~/.canary`.

No `jq`. The binary speaks JSON.

### what the bird says
On a state **transition** — and only then — the bird may say one line, in the slot
right of its beak where the note glyph otherwise sits:

```
▐ O ▌>  ♪                              silent (the default)
▐ - ▌>  ⌐ sliding. slowly. audibly.    speaking
▐ x ▌v                                 dead, and done talking
```

Most transitions produce nothing: roughly two thirds are silence on purpose. The
lines live in [`phrases/en/`](phrases/en) as plain `.txt` files, one phrase per
line, and the rarest ones are gated on **recovering**, never on hours logged — a
tool about fatigue must never pay you to keep working. The reasoning, the voice
rules, and the whole corpus are in [VOICE.md](VOICE.md); how to add a line is in
[CONTRIBUTING.md](CONTRIBUTING.md).

```sh
CANARY_QUIET=1           # bird and note glyph only, no phrases
CANARY_ASCII=1           # ASCII substitutes for ⌐ and ♪
CANARY_RESERVE_COLS=0    # cells to book on the bird's row for whatever shares it
CANARY_PHRASE_DIR=...    # corpus root, overriding the copy inside the binary
                         # (~/.canary/phrases is picked up automatically if it exists)
```

See a line drawn before you open a PR:

```sh
canary preview --state worn --note falling
canary preview --state fresh --phrase "some candidate line"
```

The corpus is compiled into the binary with `go:embed`, so the bird cannot lose
its voice in transit — the packaging has no chance to forget it. Point
`CANARY_PHRASE_DIR` at a checkout to iterate on lines without rebuilding.

## uninstall
```sh
sh uninstall.sh
```
bird gone. rc cleaned. `~/.canary` removed.

## shell
zsh, bash, fish. UTF-8 terminal required. macOS and Linux, amd64 and arm64.

## hacking
Go 1.24+, one dependency (`runewidth`, for cell widths).
```sh
go test ./...                     # everything, including the corpus linter
go build -o canary ./cmd/canary
bash test_install_uninstall.sh    # the installer, in a throwaway $HOME
shellcheck -x install.sh uninstall.sh test_install_uninstall.sh
```
CI runs all of it on ubuntu + macOS, and drives the printed hook in a real zsh,
bash and fish to check the bird actually draws above a prompt.

Where things live:

| | |
|---|---|
| `internal/fatigue` | the score: time curve, circadian shape, bands, the dead-bird demotion |
| `internal/history` | daily peaks, multi-day debt, the night streak |
| `internal/session` | signals, from Claude Code's transcript or the shell state file |
| `internal/state`   | the shell-prompt bird's own counters |
| `internal/phrase`  | what the bird says, and when it says nothing |
| `internal/render`  | the art, the stat row, and fitting a line to the cells left |
| `cmd/canary`       | the subcommands, and the shell hooks it prints |

`internal/fatigue/parity_test.go` runs the original shell arithmetic in bash and
diffs it against the Go, every minute from 0 to 1500. The shell implementation
is gone; its numbers are still the specification.

## roadmap
Where the bird is going. Shipped is shipped; the rest is intent, not a promise.

| | | |
|---|---|---|
| **v0.6** | released | five bands, circadian curve, multi-day debt, shell + Claude Code statusline |
| **v0.7** | released | the bird speaks — `phrases/en/**`, transition-gated encounters, health-gated rarity, `canary preview` |
| **v1.0** | **this release** | one Go binary with the corpus in `go:embed` — no `jq`, no shell, correct cell widths via `runewidth`; the corpus linter runs in CI; one state file per person instead of one per terminal |
| next | — | true shuffle without replacement; triggers detected in code (`no-tests`, `uncommitted`, `same-file`, `compacted`); notes that move during a real break, and only then; `mine/` untranslated lines; `ephemeral/` quarantined by year |

1.0 does not mean finished. It means the shape stopped moving: one binary, one
score, one state format, and a corpus anyone can add a line to without reading
Go. The design the phrases implement is written down in [VOICE.md](VOICE.md),
including the parts not built yet. Two rules there are load-bearing and will not be traded
away: the rarest lines are gated on **recovering, never on hours logged**, and there
is no counter, no dex, no "12/40 seen" — a collection UI turns a tool about fatigue
into something that pays you to keep working.

## license
MIT. see LICENSE.
