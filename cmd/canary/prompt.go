package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/thousandflowers/canary/internal/config"
	"github.com/thousandflowers/canary/internal/fatigue"
	"github.com/thousandflowers/canary/internal/render"
	"github.com/thousandflowers/canary/internal/state"
)

// promptScore is the shell-prompt bird's number: this session's activity, run
// through the time-of-day curve.
//
// No multi-day debt here, deliberately. The prompt bird has always scored the
// session in front of you; debt belongs to the status row, which is where the
// bird has room to explain itself with a `d12` and a nights line. A prompt that
// opened dead because of Tuesday, with no way to see why, would read as broken.
func promptScore(cfg config.Config, st state.State, now time.Time) int {
	raw := fatigue.ShellRaw(st.ActiveSeconds/60, st.PromptCount, st.AvgLen())
	return fatigue.ApplyCircadian(raw, now.Hour(), cfg.NightMult)
}

// runPrompt is the per-prompt draw. Silence is cheap and a prompt hook must
// never print an error: whatever went wrong, the person is trying to run a
// command, not debug their bird.
func runPrompt(cfg config.Config) int {
	if cfg.Disabled {
		return 0
	}
	st := loadOrReset(cfg)
	score := promptScore(cfg, st, time.Now())
	if score < cfg.MinScore {
		return 0 // below the threshold the bird stays out of your way
	}
	fmt.Print(render.Prompt(fatigue.BandFor(score), score, cfg.ShowScore))
	return 0
}

// runStatus is `canary` with no arguments: the same bird, asked for on purpose,
// so the score comes along whether or not CANARY_SHOW_SCORE is set.
func runStatus(cfg config.Config) int {
	st := loadOrReset(cfg)
	score := promptScore(cfg, st, time.Now())
	fmt.Print(render.Prompt(fatigue.BandFor(score), score, true))
	return 0
}

func runScore(cfg config.Config) int {
	st := loadOrReset(cfg)
	fmt.Println(promptScore(cfg, st, time.Now()))
	return 0
}

// runRecord accrues one command. The shell hook calls this before the command
// runs, which is why it takes the text rather than timing anything itself.
//
// `--` ends the flags: a recorded command starts with a dash often enough
// (`-la`, a typo, a heredoc line) that guessing would drop those from the
// count.
func runRecord(cfg config.Config, args []string) int {
	if cfg.Disabled {
		return 0
	}
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	cmd := strings.TrimSpace(strings.Join(args, " "))
	if cmd == "" {
		return 0 // a bare Enter is not work
	}

	st, _ := state.Load(cfg.StateFile)
	st = st.Record(cmd, int(time.Now().Unix()), cfg.IdleThreshold)
	// A failed write costs one command in the count, which is not worth a line
	// of noise between you and the command you actually typed.
	_ = state.Save(cfg.StateFile, st)
	return 0
}

func runReset(cfg config.Config) int {
	if err := reset(cfg); err != nil {
		return fail(err)
	}
	fmt.Println("canary: reset")
	return runStatus(cfg)
}

// reset zeroes the session. LastActive starts at now, so the first command
// after a reset accrues nothing rather than charging you for the gap since the
// last one.
func reset(cfg config.Config) error {
	return state.Save(cfg.StateFile, state.State{LastActive: int(time.Now().Unix())})
}

// loadOrReset honours CANARY_RESET=1, which the shell bird read on its next
// prompt. It survives because it is what the dead bird's own nudge used to
// name, and because `export CANARY_RESET=1` is muscle memory for anyone who
// has been running canary for a while.
func loadOrReset(cfg config.Config) state.State {
	if v := os.Getenv("CANARY_RESET"); v != "" && v != "0" {
		_ = reset(cfg)
		return state.State{LastActive: int(time.Now().Unix())}
	}
	st, _ := state.Load(cfg.StateFile)
	return st
}
