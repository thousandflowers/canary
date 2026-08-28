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
// else on the screen.
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
// (4 bands x (speak + move)) + dead — about sixteen seconds, which is as long
// as anybody watches a recording in a README.
const (
	demoSpeak  = 9 // two seconds: long enough to read a line, not to reread it
	demoMove   = 6 // and enough frames to see which way the note is going
	demoSilent = 5 // the dead bird's silence, which is the point of it
)

// demoTick is a variable so the tests can play the whole thing instantly.
var demoTick = 220 * time.Millisecond

// runDemo plays the sequence, then leaves the last frame on screen.
func runDemo(cfg config.Config, args []string) int {
	if len(args) != 0 {
		fmt.Fprintln(os.Stderr, "usage: canary demo")
		return 2
	}
	play(os.Stdout, demoSequence(corpus(cfg), dice, cfg), demoTick)
	return 0
}

// play draws each frame over the last one.
//
// The cursor goes up two rows and every row is cleared before it is rewritten,
// because a phrase is not always shorter than the one it replaces. The cursor
// itself is hidden: it would sit blinking in the middle of the bird.
func play(w io.Writer, frames []string, tick time.Duration) {
	fmt.Fprint(w, "\033[2J\033[H\033[?25l") // a clean screen, no cursor
	defer fmt.Fprint(w, "\033[?25h")

	for i, frame := range frames {
		if i > 0 {
			fmt.Fprint(w, "\033[2A") // back up over the two rows just drawn
		}
		for _, row := range strings.Split(frame, "\n") {
			fmt.Fprint(w, "\033[2K"+row+"\n")
		}
		time.Sleep(tick)
	}
}

// demoSequence builds every frame up front. Pure, and therefore testable: the
// player below is a loop with a sleep in it and nothing to get wrong.
//
// Each band speaks once and then moves. The order is the order the bird
// actually walks: fresh through dead, which is also the order of the five
// labelled points of the sleepiness scale the bands are taken from.
func demoSequence(c phrase.Corpus, r phrase.Rand, cfg config.Config) []string {
	var frames []string
	tick := int64(0)

	draw := func(band fatigue.Band, text string, animate bool) {
		frames = append(frames, render.Bird(render.Status{
			Band:    band,
			Phrase:  text,
			Animate: animate,
			Epoch:   tick,
			ASCII:   cfg.ASCII,
			Columns: cfg.Columns,
			Reserve: cfg.ReserveCols,
		}))
		tick++
	}

	for _, band := range []fatigue.Band{fatigue.Fresh, fatigue.Chirpy, fatigue.Tired, fatigue.Worn} {
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
