package phrase

import (
	"time"

	"github.com/thousandflowers/canary/internal/fatigue"
)

// The encounter rates from VOICE.md §4. Roughly two thirds of eligible moments
// produce nothing at all, and that silence is the feature: a bird that talks on
// every transition is a notification, which is the thing canary exists not to
// be.
const (
	SilenceRate  = 65  // percent of eligible transitions that stay quiet
	UncommonOdds = 8   // 1 in N of the remainder
	RareOdds     = 40  // 1 in N, and only when the person is recovering
	UltraOdds    = 300 // 1 in N, and only when its one condition holds
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

// UltraSession is the session of the day that earns its own line.
const UltraSession = 7

// slotAttempts is how many times a template that cannot be filled is replaced
// with another draw before the bird gives up and says nothing. A line with a
// hole in it is never printed.
const slotAttempts = 3

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

	// Triggers are the observations detected in code this refresh, most
	// interesting first. Adding a file to phrases/triggers/ does nothing
	// without a detector here — see CONTRIBUTING.
	Triggers []string

	// Now drives the two things that are functions of the clock rather than of
	// you: which ephemeral year is still readable, and the ultra conditions.
	Now time.Time
	// SessionCount is how many Claude Code sessions today has seen.
	SessionCount int
	// MineSeen is whether an untranslated line has already been drawn this
	// session. The effect works once; as a standing mode it becomes noise.
	MineSeen bool

	// Slots fill any {template} the drawn line uses.
	Slots Slots
}

// Draw is what the bird decided to say, and enough about where it came from for
// the caller to remember it.
type Draw struct {
	Text string
	Tier string // common, uncommon, rare, ultra — empty when silent
	Mine bool   // the line was untranslated, so the session has had its one
}

// Pick returns the line the bird says, or an empty Draw for silence.
//
// recent is the last handful of lines already used, and bag is the shuffle
// state: sampling is without replacement, because a plain random choice over
// thirty lines repeats within about seven draws and the rarity dies the first
// time somebody sees the same rare line twice in an evening.
func Pick(c Corpus, ctx Context, r Rand, bag *Bag, recent []string) Draw {
	// Dead bypasses the roll on purpose. Its single line, and the silence that
	// follows it, is the loudest thing this tool says (VOICE.md rule 9).
	if ctx.Band == fatigue.Dead {
		lines := c.Lines(c.In("states/dead.txt"))
		if len(lines) == 0 {
			return Draw{}
		}
		return Draw{Text: lines[0], Tier: "common"}
	}
	if r.IntN(100) < SilenceRate {
		return Draw{}
	}

	// One emptiness check, not one per stage: an empty pool and a pool whose
	// files hold nothing are the same answer — silence.
	tier, files := poolFor(c, ctx, r)
	lines := c.Lines(files...)
	if len(lines) == 0 {
		return Draw{}
	}

	mineLines := map[string]bool{}
	if tier == "rare" && mineAllowed(ctx) {
		for _, l := range c.Lines(c.Files("mine")...) {
			mineLines[l] = true
		}
	}

	// A template whose slots cannot be filled is skipped and another is drawn,
	// rather than printed with a hole in it.
	for i := 0; i < slotAttempts; i++ {
		// Draw always returns a line here: the pool is not empty, and the bag
		// falls back to a recently used line rather than to nothing.
		raw := bag.Draw(poolKey(files), lines, r, recent)
		if text := Resolve(raw, ctx.Slots); text != "" {
			return Draw{Text: text, Tier: tier, Mine: mineLines[raw]}
		}
	}
	return Draw{}
}

// poolFor rolls a tier and assembles the files it may draw from.
//
// The rolls run rarest first so each tier's odds are its own: checking common
// first would make every later roll conditional on it and quietly change every
// number in VOICE.md's table.
func poolFor(c Corpus, ctx Context, r Rand) (string, []string) {
	if f := ultraFile(c, ctx); f != "" && r.IntN(UltraOdds) == 0 {
		return "ultra", []string{f}
	}
	if rareAllowed(ctx) && r.IntN(RareOdds) == 0 {
		return "rare", rarePool(c, ctx)
	}
	if r.IntN(UncommonOdds) == 0 {
		// worn drops lore entirely (VOICE.md §6), so an uncommon roll there
		// degrades to silence rather than borrowing from a tier it cannot have.
		if ctx.Band == fatigue.Worn {
			return "", nil
		}
		return "uncommon", uncommonPool(c, ctx)
	}
	return "common", commonPool(c, ctx)
}

// rareAllowed is the gate the whole design hangs on: the rarest lines appear
// after a real break and in recovering states, never at hour six.
//
// An encounter system rewards whatever it takes to trigger an encounter, so in
// a tool about fatigue, gating rarity on session length would pay people to
// keep working. Chasing these means resting. That inversion is not optional.
func rareAllowed(ctx Context) bool {
	switch ctx.Band {
	case fatigue.Fresh, fatigue.Chirpy:
		return true
	case fatigue.Tired:
		return ctx.OnBreak || ctx.Note == "rising"
	default:
		return false
	}
}

// mineAllowed: rare tier only, low states only, never twice in a session.
func mineAllowed(ctx Context) bool {
	if ctx.MineSeen {
		return false
	}
	return ctx.Band == fatigue.Fresh || ctx.Band == fatigue.Chirpy
}

func rarePool(c Corpus, ctx Context) []string {
	pool := []string{c.In("lore/cage.txt"), c.In("lore/facts.txt")}
	if ctx.Band != fatigue.Tired {
		// A tired bird earns lore, not the outside world.
		pool = append(pool,
			c.In("worldly/outside.txt"),
			c.In("worldly/culture.txt"),
			c.In("worldly/subcultures.txt"))
	}
	if mineAllowed(ctx) {
		pool = append(pool, c.Files("mine")...)
	}
	return present(c, pool)
}

func uncommonPool(c Corpus, ctx Context) []string {
	// A trigger is the most specific thing the bird can say, so when one is
	// live it takes the tier rather than sharing it with lore about 1986.
	for _, t := range ctx.Triggers {
		if f := c.In("triggers/" + t + ".txt"); c.Has(f) {
			return []string{f}
		}
	}
	pool := []string{c.In("lore/job.txt"), c.In("lore/detector.txt")}
	// Ephemeral lines live two years and then stop being read. Uncommon rather
	// than rare on purpose: a line that expires has to be seen inside its life.
	pool = append(pool, c.Ephemeral(ctx.Now)...)
	return present(c, pool)
}

func commonPool(c Corpus, ctx Context) []string {
	band := string(ctx.Band)
	var pool []string
	// state+note first, plain state as the fallback; an empty file keeps the
	// search falling through rather than winning and going mute.
	for _, f := range []string{
		c.In("states/" + band + "+" + ctx.Note + ".txt"),
		c.In("states/" + band + ".txt"),
	} {
		if c.Has(f) {
			pool = []string{f}
			break
		}
	}
	// worn is the actionable band. It says one thing about being worn and does
	// not chat about the weather or how long you were gone.
	if ctx.Band != fatigue.Worn {
		if f := c.In("notes/" + ctx.Note + ".txt"); c.Has(f) {
			pool = append(pool, f)
		}
		if ctx.Return != "" {
			if f := c.In("returns/" + ctx.Return + ".txt"); c.Has(f) {
				pool = append(pool, f)
			}
		}
	}
	if ctx.Late {
		if f := c.In("time/late.txt"); c.Has(f) {
			pool = append(pool, f)
		}
	}
	return pool
}

// ultraFile returns the one file whose condition holds right now, if any.
//
// Every condition is read from the clock or from a count the bird already
// keeps. Nearly nobody meets one, which is the entire point: the people who do
// take a screenshot.
func ultraFile(c Corpus, ctx Context) string {
	var name string
	switch {
	case ctx.Now.Month() == time.December && ctx.Now.Day() == 25:
		name = "christmas"
	case ctx.Now.Hour() == 4 && ctx.Now.Minute() == 4:
		name = "four-oh-four"
	case ctx.SessionCount == UltraSession:
		name = "seventh-session"
	default:
		return ""
	}
	if f := c.In("ultra/" + name + ".txt"); c.Has(f) {
		return f
	}
	return ""
}

// present drops the files that do not exist or hold nothing, so a pool's key —
// and therefore its shuffle — does not change every time an optional file is
// added to the repo.
func present(c Corpus, files []string) []string {
	out := files[:0:0]
	for _, f := range files {
		if c.Has(f) {
			out = append(out, f)
		}
	}
	return out
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

func contains(hay []string, needle string) bool {
	for _, s := range hay {
		if s == needle {
			return true
		}
	}
	return false
}
