package main

import (
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"time"

	canary "github.com/thousandflowers/canary"
	"github.com/thousandflowers/canary/internal/config"
	"github.com/thousandflowers/canary/internal/fatigue"
	"github.com/thousandflowers/canary/internal/history"
	"github.com/thousandflowers/canary/internal/phrase"
	"github.com/thousandflowers/canary/internal/render"
	"github.com/thousandflowers/canary/internal/session"
)

// secondsPerDay converts unix seconds to the epoch day the history file keys on.
const secondsPerDay = 86400

// deadScore is the score past which today counts as a night past your limit.
const deadScore = 90

// runStatusline draws Claude Code's status row.
//
// Dual mode, as the script was: Claude Code pipes its session JSON on stdin
// every refresh, and when nothing is piped in the bird falls back to the state
// file the shell prompt keeps, so both surfaces tell the same story.
//
// It exits 0 on every path. Claude Code chains status line commands with `;`,
// so a non-zero exit here leaks out as the whole chain's status, and `brew
// test` fails on it.
func runStatusline(cfg config.Config) int {
	if cfg.Disabled {
		return 0
	}
	now := time.Now()

	sig, ok := gatherSignals(cfg)
	if !ok {
		// No session and no state file: nothing true to report, and a fresh
		// bird here would be a lie.
		return 0
	}

	var raw int
	if sig.StatName == "p" {
		raw = fatigue.ShellRaw(sig.Minutes, sig.Turns, sig.AvgLen)
	} else {
		raw = fatigue.ClaudeCodeRaw(sig.Minutes, sig.Turns, sig.Errors, sig.Reps, cfg.ErrWeight, cfg.RepWeight)
	}
	raw = fatigue.ApplyCircadian(raw, now.Hour(), cfg.NightMult)

	today := int(now.Unix() / secondsPerDay)
	entries, _ := history.Load(cfg.HistoryFile)
	past := history.Summarize(entries, today, cfg.DebtMax)

	score := fatigue.Cap(raw + past.Debt)
	nights := past.Nights
	if score > deadScore {
		nights++ // today extends the streak
	}

	// Today's peak is stored pre-debt, so yesterday's debt never compounds into
	// tomorrow's. Written before the quiet threshold below: a session that
	// stayed under CANARY_MIN_SCORE still happened.
	_ = history.RecordPeak(cfg.HistoryFile, today, raw)

	if score < cfg.MinScore {
		return 0
	}

	band := fatigue.Demote(fatigue.BandFor(score), raw, past.Personal, nights, cfg.DeadAbsolute)
	text := speak(cfg, band, score, sig.Minutes, now)

	fmt.Print(render.Statusline(render.Status{
		Band:      band,
		Minutes:   sig.Minutes,
		Turns:     sig.Turns,
		StatName:  sig.StatName,
		Errors:    sig.Errors,
		Debt:      past.Debt,
		Score:     score,
		Nights:    nights,
		Phrase:    text,
		ShowScore: cfg.ShowScore,
		ASCII:     cfg.ASCII,
		Columns:   cfg.Columns,
		Reserve:   cfg.ReserveCols,
	}))
	return 0
}

// gatherSignals picks the mode from what is on stdin.
func gatherSignals(cfg config.Config) (session.Signals, bool) {
	if input := readStdin(); session.IsClaudeCode(input) {
		return session.FromClaudeCode(input), true
	}
	return session.FromShellState(cfg.StateFile)
}

// readStdin returns whatever was piped in, or nothing when stdin is a terminal.
// Reading from a tty would block the status row forever waiting for a person to
// type into a script they cannot see.
func readStdin() []byte {
	fi, err := os.Stdin.Stat()
	if err != nil || fi.Mode()&os.ModeCharDevice != 0 {
		return nil
	}
	b, err := io.ReadAll(os.Stdin)
	if err != nil {
		return nil
	}
	return b
}

// speak decides what the bird says, and remembers it.
//
// The trigger is a state TRANSITION, never the clock: a line is drawn when the
// band changes, held for a minute so it can be read across the several refreshes
// a second Claude Code asks for, and then the bird goes quiet again rather than
// finding something new to say.
func speak(cfg config.Config, band fatigue.Band, score, minutes int, now time.Time) string {
	mem := phrase.LoadMemory(cfg.PhraseState)

	gap := -1
	if mem.TS > 0 {
		gap = int(now.Unix() - mem.TS)
	}

	text, phTS := "", mem.PhTS
	if !cfg.Quiet {
		switch {
		case string(band) != mem.Band:
			ctx := phrase.Classify(band, score, mem.Score, mem.Known, gap, now.Hour(), minutes)
			text = phrase.Pick(corpus(cfg), ctx, globalRand{}, phrase.LoadRecent(cfg.RecentFile))
			phTS = now.Unix()
			if text != "" {
				_ = phrase.AppendRecent(cfg.RecentFile, text)
			}
		case mem.Phrase != "" && now.Unix()-mem.PhTS <= phrase.HoldSeconds:
			text = mem.Phrase // still inside the hold, keep it on screen
		}
	}

	_ = phrase.SaveMemory(cfg.PhraseState, phrase.Memory{
		Band:   string(band),
		Score:  score,
		TS:     now.Unix(),
		PhTS:   phTS,
		Phrase: text,
	})
	return text
}

// corpus is the embedded phrase tree, unless a directory overrides it.
func corpus(cfg config.Config) phrase.Corpus {
	if cfg.PhraseDir != "" {
		return phrase.FromDir(cfg.PhraseDir)
	}
	return phrase.FromFS(canary.Corpus)
}

// globalRand is the process-wide generator, which math/rand/v2 seeds for us.
// The phrase package takes the interface so a test can hand it a sequence
// instead of waiting for a 1-in-40 branch to show up.
type globalRand struct{}

func (globalRand) IntN(n int) int { return rand.IntN(n) }
