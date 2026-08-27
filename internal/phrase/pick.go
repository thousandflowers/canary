package phrase

import (
	"github.com/thousandflowers/canary/internal/fatigue"
)

// The encounter rates from VOICE.md §4. Roughly two thirds of eligible moments
// produce nothing at all, and that silence is the feature: a bird that talks on
// every transition is a notification, which is the thing canary exists not to
// be.
const (
	SilenceRate  = 65 // percent of eligible transitions that stay quiet
	UncommonOdds = 8  // 1 in N of the remainder
	RareOdds     = 40 // 1 in N, and only when the person is recovering
)

// Thresholds for how long you were away, in seconds.
const (
	gapShort = 60
	gapBreak = 180
	gapLong  = 3600
	gapDays  = 86400
)

// lateHour and lateMinutes are when "late" stops being a time and starts being
// a description of you.
const (
	lateHour    = 6
	lateMinutes = 240
)

// Rand is the slice of math/rand/v2 this package needs, so a test can hand it a
// sequence instead of hoping an unlikely branch shows up.
type Rand interface{ IntN(n int) int }

// Context is everything outside the score that steers what gets said.
type Context struct {
	Band fatigue.Band
	// Note is the HUMAN's trend, not the score's: a score going up means the
	// person is going down. worn+rising is someone pulling it back.
	Note string // rising, falling, steady, unknown
	// Return is how long the terminal sat idle before this moment.
	Return string // "", short, from-break, long, days
	// OnBreak means the gap was long enough to have been a real break, which
	// is one of the two ways to earn a rare line.
	OnBreak bool
	Late    bool
}

// Classify derives the context from the two numbers the bird remembers.
//
// hasPrev distinguishes "the score has not moved" from "there is no previous
// score", which are different states and used to collapse into steady.
func Classify(band fatigue.Band, score, prevScore int, hasPrev bool, gap, hour, minutes int) Context {
	ctx := Context{Band: band, Note: "unknown"}
	switch {
	case !hasPrev:
		ctx.Note = "unknown"
	case score > prevScore:
		ctx.Note = "falling"
	case score < prevScore:
		ctx.Note = "rising"
	default:
		ctx.Note = "steady"
	}

	switch {
	case gap >= gapDays:
		ctx.Return = "days"
	case gap >= gapLong:
		ctx.Return = "long"
	case gap >= gapBreak:
		ctx.Return = "from-break"
	case gap >= gapShort:
		ctx.Return = "short"
	}
	ctx.OnBreak = gap >= gapBreak
	ctx.Late = hour <= lateHour || minutes >= lateMinutes
	return ctx
}

// Pick returns the line the bird says, or "" for silence.
//
// recent is the last handful of lines already used; a candidate that repeats
// one gets a single redraw. That is not a shuffle without replacement and does
// not pretend to be — it removes the one visible defect, the same line twice
// running, for the cost of a ten-line file.
func Pick(c Corpus, ctx Context, r Rand, recent []string) string {
	// Dead bypasses the roll on purpose. Its single line, and the silence that
	// follows it, is the loudest thing this tool says (VOICE.md rule 9).
	if ctx.Band == fatigue.Dead {
		return draw(c.Lines("states/dead.txt"), r)
	}
	if r.IntN(100) < SilenceRate {
		return ""
	}

	pool := poolFor(c, ctx, tierFor(ctx, r))
	lines := c.Lines(pool...)
	if len(lines) == 0 {
		return ""
	}

	cand := draw(lines, r)
	if contains(recent, cand) {
		cand = draw(lines, r)
	}
	return cand
}

type tier int

const (
	common tier = iota
	uncommon
	rare
)

// tierFor rolls the tier. Rare is gated on HEALTH, never on time: fresh and
// chirpy always qualify, tired qualifies only while recovering — coming back
// from a break, or with a score that is actually falling — and worn never
// does. An encounter gated on session length would pay you to keep working.
func tierFor(ctx Context, r Rand) tier {
	rareOK := false
	switch ctx.Band {
	case fatigue.Fresh, fatigue.Chirpy:
		rareOK = true
	case fatigue.Tired:
		rareOK = ctx.OnBreak || ctx.Note == "rising"
	}
	if rareOK && r.IntN(RareOdds) == 0 {
		return rare
	}
	if r.IntN(UncommonOdds) == 0 {
		return uncommon
	}
	return common
}

// poolFor assembles the files a tier may draw from.
func poolFor(c Corpus, ctx Context, t tier) []string {
	band := string(ctx.Band)
	switch t {
	case uncommon:
		// worn drops lore entirely (VOICE.md §6), so an uncommon roll there
		// degrades to silence rather than borrowing from a tier it cannot have.
		if ctx.Band == fatigue.Worn {
			return nil
		}
		return []string{"lore/job.txt", "lore/detector.txt"}

	case rare:
		pool := []string{"lore/cage.txt", "lore/facts.txt"}
		if ctx.Band != fatigue.Tired {
			pool = append(pool, "worldly/outside.txt", "worldly/culture.txt", "worldly/subcultures.txt")
		}
		return pool

	default:
		var pool []string
		// state+note first, plain state as the fallback; an empty file keeps
		// the search falling through rather than winning and going mute.
		for _, f := range []string{
			"states/" + band + "+" + ctx.Note + ".txt",
			"states/" + band + ".txt",
		} {
			if c.Has(f) {
				pool = []string{f}
				break
			}
		}
		// worn is the actionable band. It says one thing about being worn and
		// does not chat about the weather or how long you were gone.
		if ctx.Band != fatigue.Worn {
			if f := "notes/" + ctx.Note + ".txt"; c.Has(f) {
				pool = append(pool, f)
			}
			if ctx.Return != "" {
				if f := "returns/" + ctx.Return + ".txt"; c.Has(f) {
					pool = append(pool, f)
				}
			}
		}
		if ctx.Late && c.Has("time/late.txt") {
			pool = append(pool, "time/late.txt")
		}
		return pool
	}
}

func draw(lines []string, r Rand) string {
	if len(lines) == 0 {
		return ""
	}
	return lines[r.IntN(len(lines))]
}

func contains(hay []string, needle string) bool {
	for _, s := range hay {
		if s == needle {
			return true
		}
	}
	return false
}
