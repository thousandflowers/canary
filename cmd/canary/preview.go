package main

import (
	"fmt"
	"os"

	"github.com/thousandflowers/canary/internal/config"
	"github.com/thousandflowers/canary/internal/fatigue"
	"github.com/thousandflowers/canary/internal/render"
)

// previewScores put each band in the middle of its range, so a previewed line
// is drawn by the same code path the real bird uses rather than a mock of it.
var previewScores = map[fatigue.Band]int{
	fatigue.Fresh:  10,
	fatigue.Chirpy: 35,
	fatigue.Tired:  60,
	fatigue.Worn:   85,
	fatigue.Dead:   95,
}

const previewUsage = `usage: canary preview [--state fresh|chirpy|tired|worn|dead]
                      [--note rising|falling|steady|unknown] [--phrase TEXT]`

// runPreview draws a phrase without waiting for the real bird to reach that
// state. Contributors who do not code cannot picture their line until they see
// it drawn, and a corpus PR should not require a five-hour session to review.
//
// Nothing is persisted: no state, no history, no recent queue.
func runPreview(cfg config.Config, args []string) int {
	band := fatigue.Fresh
	note := "unknown"
	text := ""

	for len(args) > 0 {
		flag := args[0]
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, previewUsage)
			return 2
		}
		value := args[1]
		args = args[2:]

		switch flag {
		case "--state":
			b, ok := parseBand(value)
			if !ok {
				fmt.Fprintf(os.Stderr, "canary: unknown state: %s\n", value)
				return 2
			}
			band = b
		case "--note":
			note = value
		case "--phrase":
			text = value
		default:
			fmt.Fprintln(os.Stderr, previewUsage)
			return 2
		}
	}

	if text == "" {
		// The state's own files only. A preview that could wander into lore or
		// worldly would show a line the state cannot actually produce.
		c := corpus(cfg)
		lines := c.Lines(c.In("states/"+string(band)+"+"+note+".txt"), c.In("states/"+string(band)+".txt"))
		if len(lines) > 0 {
			text = lines[dice.IntN(len(lines))]
		}
	}

	fmt.Print(render.Statusline(render.Status{
		Band:      band,
		StatName:  "t",
		Score:     previewScores[band],
		Phrase:    text,
		ShowScore: cfg.ShowScore,
		ASCII:     cfg.ASCII,
		Columns:   cfg.Columns,
		Reserve:   cfg.ReserveCols,
	}))
	fmt.Println()
	return 0
}

func parseBand(s string) (fatigue.Band, bool) {
	for _, b := range []fatigue.Band{fatigue.Fresh, fatigue.Chirpy, fatigue.Tired, fatigue.Worn, fatigue.Dead} {
		if string(b) == s {
			return b, true
		}
	}
	return "", false
}
