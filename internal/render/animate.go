package render

import "github.com/thousandflowers/canary/internal/fatigue"

// Notes move only during a real break, while the bird recovers. Static during
// work, always.
//
// The only pretty thing this tool does requires you to stop working to see it,
// which is the same inversion the rare phrases use: nothing here can be earned
// by grinding.
//
// The frame comes from the clock, never from a counter. The status row is
// redrawn several times a second and cancelled mid-run, so anything persisted
// and incremented drifts within minutes — and a stateless frame costs nothing
// to compute.

// slowFactor is the cadence divisor for the deeper bands. A worn bird moves at
// a third of the speed of a fresh one, and the slowness reads on its own
// without a single word.
const slowFactor = 3

// Every frame in a pattern must occupy the same number of cells. `···` is a
// real frame, not "nothing": a shorter one would shift whatever shares the row
// — caveman's badge, most often — sideways on every tick.
var (
	driftFrames    = []string{"♪··", "·♪·", "··♫", "···"}
	mutateFrames   = []string{"♪··", "♫··", "♬··", "♪··"}
	mixedFrames    = []string{"♪··", "·♪·", "·♫·", "·♪·", "♪··", "···"}
	hesitantFrames = []string{"·", "♪", "·", "♪"}

	// Musical glyphs are East Asian Ambiguous width and render double in some
	// terminals. CANARY_ASCII=1 is the way out, and with another segment on the
	// same row, alignment beats typography.
	driftASCII    = []string{"o..", ".o.", "..O", "..."}
	mutateASCII   = []string{"o..", "O..", "0..", "o.."}
	mixedASCII    = []string{"o..", ".o.", ".O.", ".o.", "o..", "..."}
	hesitantASCII = []string{".", "o", ".", "o"}
)

// Frames is the pattern a band recovers to.
//
// A fresh bird gets the pattern that rises, hesitates and falls; a chirpy one
// changes note in place; a tired one drifts; a worn one has two cells of
// hesitation left. Dead gets nothing: it has said its line and it is done
// talking, and an animated corpse would be a joke at the wrong moment.
func Frames(b fatigue.Band, ascii bool) []string {
	switch b {
	case fatigue.Fresh:
		if ascii {
			return mixedASCII
		}
		return mixedFrames
	case fatigue.Chirpy:
		if ascii {
			return mutateASCII
		}
		return mutateFrames
	case fatigue.Tired:
		if ascii {
			return driftASCII
		}
		return driftFrames
	case fatigue.Worn:
		if ascii {
			return hesitantASCII
		}
		return hesitantFrames
	default:
		return nil
	}
}

// AnimatedNote is the frame to draw at this second, or "" when the band does
// not animate.
func AnimatedNote(b fatigue.Band, epoch int64, ascii bool) string {
	frames := Frames(b, ascii)
	if len(frames) == 0 {
		return ""
	}
	tick := epoch
	if b == fatigue.Tired || b == fatigue.Worn {
		tick /= slowFactor
	}
	i := tick % int64(len(frames))
	if i < 0 {
		i += int64(len(frames))
	}
	return frames[i]
}
