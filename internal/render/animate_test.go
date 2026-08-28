package render

import (
	"testing"

	"github.com/mattn/go-runewidth"
	"github.com/thousandflowers/canary/internal/fatigue"
)

func TestEveryFrameInAPatternIsTheSameWidth(t *testing.T) {
	// `···` is a real frame, not "nothing". A shorter one would shift whatever
	// shares the row — caveman's badge, most often — sideways on every tick.
	for _, ascii := range []bool{false, true} {
		for _, band := range []fatigue.Band{fatigue.Fresh, fatigue.Chirpy, fatigue.Tired, fatigue.Worn} {
			frames := Frames(band, ascii)
			if len(frames) == 0 {
				t.Fatalf("%s has no frames (ascii=%v)", band, ascii)
			}
			want := runewidth.StringWidth(frames[0])
			for _, f := range frames {
				if got := runewidth.StringWidth(f); got != want {
					t.Errorf("%s ascii=%v: frame %q is %d cells, first frame is %d", band, ascii, f, got, want)
				}
			}
		}
	}
}

func TestTheDeadBirdDoesNotDance(t *testing.T) {
	// It has said its line and it is done talking. An animated corpse would be
	// a joke at the wrong moment.
	if got := AnimatedNote(fatigue.Dead, 12, false); got != "" {
		t.Errorf("the dead bird moved: %q", got)
	}
}

func TestTheFrameComesFromTheClock(t *testing.T) {
	// Never from a counter: the row is redrawn several times a second and
	// cancelled mid-run, so anything persisted and incremented drifts.
	frames := Frames(fatigue.Fresh, false)
	for i := 0; i < len(frames)*3; i++ {
		want := frames[i%len(frames)]
		if got := AnimatedNote(fatigue.Fresh, int64(i), false); got != want {
			t.Errorf("epoch %d: got %q, want %q", i, got, want)
		}
	}
	// The same second always draws the same frame, whoever asks.
	if AnimatedNote(fatigue.Fresh, 1000, false) != AnimatedNote(fatigue.Fresh, 1000, false) {
		t.Error("two draws of one second disagreed")
	}
}

func TestDeeperStatesMoveSlower(t *testing.T) {
	// The slowness reads on its own, without a single word.
	first := AnimatedNote(fatigue.Worn, 0, false)
	if AnimatedNote(fatigue.Worn, 1, false) != first || AnimatedNote(fatigue.Worn, 2, false) != first {
		t.Error("a worn bird changed frame every second")
	}
	if AnimatedNote(fatigue.Worn, slowFactor, false) == first {
		t.Error("a worn bird never changed frame at all")
	}
}

func TestASCIIFramesCarryNoMusicalGlyphs(t *testing.T) {
	// Musical glyphs are East Asian Ambiguous width and render double in some
	// terminals. With another segment on the same row, alignment beats
	// typography.
	for _, band := range []fatigue.Band{fatigue.Fresh, fatigue.Chirpy, fatigue.Tired, fatigue.Worn} {
		for _, f := range Frames(band, true) {
			for _, r := range f {
				if r > 127 {
					t.Errorf("%s: ASCII frame %q contains %q", band, f, r)
				}
			}
		}
	}
}

func TestTheNoteOnlyMovesOnABreak(t *testing.T) {
	// The only pretty thing this tool does requires you to stop working to see
	// it — the same inversion the rare phrases use.
	working := Statusline(Status{Band: fatigue.Fresh, Columns: 80, Epoch: 1})
	if !contains(working, "♪") {
		t.Errorf("a working bird lost its static note:\n%s", working)
	}

	resting := Statusline(Status{Band: fatigue.Fresh, Columns: 80, Epoch: 1, Animate: true})
	if resting == working {
		t.Errorf("the note did not move during a break:\n%s", resting)
	}
	if !contains(resting, AnimatedNote(fatigue.Fresh, 1, false)) {
		t.Errorf("the drawn frame is not this second's:\n%s", resting)
	}
}

func TestAPhraseStillWinsTheSlotOnABreak(t *testing.T) {
	// One slot, and a line the bird actually has to say outranks the dancing.
	out := Statusline(Status{Band: fatigue.Fresh, Columns: 80, Animate: true, Phrase: "you stopped. good. i'm dancing now."})
	if !contains(out, "i'm dancing now.") {
		t.Errorf("the phrase lost its slot to the animation:\n%s", out)
	}
}

func contains(hay, needle string) bool {
	for i := 0; i+len(needle) <= len(hay); i++ {
		if hay[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
