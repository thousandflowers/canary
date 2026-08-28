package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/thousandflowers/canary/internal/config"
	"github.com/thousandflowers/canary/internal/fatigue"
	"github.com/thousandflowers/canary/internal/phrase"
	"github.com/thousandflowers/canary/internal/render"
)

// `canary demo` plays the bird through its five states, in place, with nothing
// else on the screen: the status row Claude Code shows, the bird under it, and
// whatever it has to say beside the beak.
//
// It exists because the bird is the whole product and it cannot be shown any
// other way: a screenshot has no wilting in it, and a recording of somebody
// typing commands into a shell shows the commands, not the canary. Everything
// here is drawn by the same code the real bird uses — the same art, the same
// slot beside the beak, the same animation frames — so the demo cannot drift
// away from the thing it is demonstrating.
//
// It writes nothing: no state, no history, no shuffle. Watching the bird must
// not age it.

// The demo's pace, in ticks. A tick is demoTick long, and the whole sequence is
// (4 bands x (speak + move)) + dead — about twenty-two seconds.
//
// Slower than it reads on paper, deliberately: a line has to be read once
// without effort, and a note has to be watched long enough to see which way it
// is going. A recording nobody can follow is a recording of nothing.
const (
	demoSpeak  = 9 // ~2.7s, which is a comfortable read of a full line
	demoMove   = 6 // ~1.8s, enough frames to see the note rise and fall
	demoSilent = 5 // the dead bird's silence, which is the point of it
)

// demoTick is a variable so the tests can play the whole thing instantly.
var demoTick = 300 * time.Millisecond

// demoSession is what the stat row says in each band.
//
// Real numbers, not a counter: every pair here actually produces the band it is
// listed under, and a test asserts exactly that. A demo whose numbers do not
// add up teaches the wrong thing about the only figures canary ever prints.
var demoSession = map[fatigue.Band]struct{ Minutes, Turns int }{
	fatigue.Fresh:  {8, 3},
	fatigue.Chirpy: {45, 14},
	fatigue.Tired:  {110, 31},
	fatigue.Worn:   {200, 52},
	fatigue.Dead:   {300, 78},
}

const demoUsage = "usage: canary demo [--state fresh|chirpy|tired|worn|dead]"

// runDemo plays the sequence, then leaves the last frame on screen.
//
// With --state it plays one band and stops, which is what a small example of a
// single state wants to be. That mode draws no status row: one state on its own
// is not a session, and reporting minutes and turns for one would be inventing
// a session that is not happening.
func runDemo(cfg config.Config, args []string) int {
	var only *fatigue.Band
	switch len(args) {
	case 0:
	case 2:
		if args[0] != "--state" {
			fmt.Fprintln(os.Stderr, demoUsage)
			return 2
		}
		band, ok := parseBand(args[1])
		if !ok {
			fmt.Fprintf(os.Stderr, "canary: unknown state: %s\n", args[1])
			return 2
		}
		only = &band
	default:
		fmt.Fprintln(os.Stderr, demoUsage)
		return 2
	}
	play(os.Stdout, demoSequence(corpus(cfg), dice, cfg, only), demoTick)
	return 0
}

// play draws each frame over the last one.
//
// The cursor goes back up over however many rows the last frame took, and every
// row is cleared before it is rewritten, because a phrase is not always shorter
// than the one it replaces. The cursor
// itself is hidden: it would sit blinking in the middle of the bird.
func play(w io.Writer, frames []string, tick time.Duration) {
	fmt.Fprint(w, "\033[2J\033[H\033[?25l") // a clean screen, no cursor
	defer fmt.Fprint(w, "\033[?25h")

	drawn := 0
	for _, frame := range frames {
		if drawn > 0 {
			fmt.Fprintf(w, "\033[%dA", drawn) // back up over the rows just drawn
		}
		rows := strings.Split(frame, "\n")
		for _, row := range rows {
			fmt.Fprint(w, "\033[2K"+row+"\n")
		}
		drawn = len(rows)
		time.Sleep(tick)
	}
}

// demoSequence builds every frame up front. Pure, and therefore testable: the
// player below is a loop with a sleep in it and nothing to get wrong.
//
// Each band speaks once and then moves. The order is the order the bird
// actually walks: fresh through dead, which is also the order of the five
// labelled points of the sleepiness scale the bands are taken from.
func demoSequence(c phrase.Corpus, r phrase.Rand, cfg config.Config, only *fatigue.Band) []string {
	var frames []string
	tick := int64(0)

	// Within a band the clock keeps moving, so the row is alive rather than a
	// screenshot: a minute every third tick, which is about the pace of the
	// real thing.
	elapsed := 0
	draw := func(band fatigue.Band, text string, animate bool) {
		session := demoSession[band]
		st := render.Status{
			Band:     band,
			Minutes:  session.Minutes + elapsed/3,
			Turns:    session.Turns,
			StatName: "t",
			Phrase:   text,
			Animate:  animate,
			Epoch:    tick,
			ASCII:    cfg.ASCII,
			Columns:  cfg.Columns,
			Reserve:  cfg.ReserveCols,
		}
		if only != nil {
			frames = append(frames, render.Bird(st))
		} else {
			frames = append(frames, render.Statusline(st))
		}
		tick++
		elapsed++
	}

	bands := []fatigue.Band{fatigue.Fresh, fatigue.Chirpy, fatigue.Tired, fatigue.Worn}
	if only != nil {
		if *only == fatigue.Dead {
			bands = nil
		} else {
			bands = []fatigue.Band{*only}
		}
	}

	for _, band := range bands {
		elapsed = 0
		line := demoLine(c, band, r)
		for i := 0; i < demoSpeak; i++ {
			draw(band, line, false)
		}
		// Then quiet, and moving: the note animation belongs to a bird that is
		// not being talked over.
		for i := 0; i < demoMove; i++ {
			draw(band, "", true)
		}
	}

	// The dead bird says its one line and then stops. No note, no movement —
	// the silence after it is the loudest thing this tool says.
	if only != nil && *only != fatigue.Dead {
		return frames
	}
	elapsed = 0
	dead := demoLine(c, fatigue.Dead, r)
	for i := 0; i < demoSpeak; i++ {
		draw(fatigue.Dead, dead, false)
	}
	for i := 0; i < demoSilent; i++ {
		draw(fatigue.Dead, "", true)
	}
	return frames
}

// demoLine takes one line from the band's own file.
//
// Not through Pick: the demo has to speak every time, and Pick is two thirds
// silence by design. It is still the real corpus, drawn from the real file.
func demoLine(c phrase.Corpus, band fatigue.Band, r phrase.Rand) string {
	lines := c.Lines(c.In("states/" + string(band) + ".txt"))
	if len(lines) == 0 {
		return ""
	}
	return lines[r.IntN(len(lines))]
}
