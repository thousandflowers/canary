// Package chrono learns when you are actually awake, so the time-of-day curve
// can be aimed at your body clock instead of a textbook one.
//
// fatigue.CircadianExcess encodes the population shape: nadir at 02:00-06:00,
// post-lunch dip, evening wake-maintenance zone. That shape is well evidenced
// and worth keeping. Its *phase* is not: a chronotype shifts by hours between
// people, and for one person it shifts with the season, the term, and whatever
// they are in the middle of. A bird tuned to someone who sleeps at midnight
// punishes a 04:00 sleeper exactly when they are sharpest and forgives them
// exactly when they are falling over.
//
// A CANARY_CHRONO_OFFSET knob would fix that once and then quietly rot, which
// is the failure mode of every hand-set calibration. So the phase is measured
// instead: 24 counters, one per hour, each recording that you were active in
// that hour, all of them decayed daily so the estimate follows you rather than
// averaging your whole life. The knob survives as an override, not as the
// mechanism.
//
// What is deliberately NOT here: any attempt to read a sensor. Die temperature
// needs root on Apple Silicon (powermetrics refuses as a normal user, and the
// SMC keys are not in ioreg or sysctl), and it measures the machine's workload
// anyway — a hot laptop at 09:00 is a render, not a tired person.
package chrono

import (
	"bufio"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/thousandflowers/canary/internal/atomicfile"
)

// Tuning. These are the numbers the estimator's behaviour actually rests on,
// named rather than inlined because moving any of them changes what the bird
// believes about you.
const (
	// Weight is what one active hour adds. Counting hours rather than commands
	// is on purpose: a 300-command burst at 15:00 is one hour of being awake at
	// 15:00, and letting it outvote a whole quiet week would be reading
	// intensity as if it were schedule.
	Weight = 16

	// decayNum/decayDen is the daily decay, 15/16 per day: a half-life near 11
	// days. Fast enough that moving your hours shows up inside a fortnight,
	// slow enough that one late night does not repaint the curve.
	decayNum = 15
	decayDen = 16

	// deadDays is when an untouched log is treated as gone rather than decayed
	// one day at a time. (15/16)^64 is under 2%, and integer truncation has
	// flattened it to zero well before that.
	deadDays = 64

	// SteadyState is what a slot settles at when you are active in that hour
	// every single day: Weight/(1-decay). It is the scale everything else is
	// read against, and what a seeded log has to be expressed in to be
	// comparable with a lived-in one.
	SteadyState = Weight * decayDen / (decayDen - decayNum)

	// maxSlot bounds a slot against a corrupted or hand-edited file. The
	// steady state for an always-active hour is Weight/(1-15/16) = 256.
	maxSlot = 4096

	// MinTotal is the evidence needed before the estimate is used at all:
	// roughly 40 recorded active hours, four good days. Below it the bird keeps
	// the literature curve, because a guess from three data points is worse
	// than the textbook it replaced.
	MinTotal = Weight * 40

	// minR is how concentrated the histogram has to be before its centre means
	// anything. R is the resultant length from circular statistics: 1.0 is all
	// activity in one hour, 0 is activity spread evenly around the clock, and a
	// clean sixteen-hour waking block scores 0.42. Below this the log describes
	// someone with no discernible schedule, and the honest answer is the
	// textbook curve rather than a confident rotation of noise.
	minR = 0.15

	// refWake and refSleep are the waking block fatigue.CircadianExcess fatigue.CircadianExcess was written
	// around: awake 07:00, asleep 23:00. That curve calls 07:00 "the tail of
	// the nadir" and 08:00-12:00 the morning peak, which is the shape of
	// someone who just got up at seven.
	//
	// They exist to be turned into a reference centre, not compared against
	// directly. Anchoring on an absolute nadir instead (the core-body-
	// temperature minimum runs about two hours before habitual waking) rotated
	// the textbook sleeper by +3 — a calibration has to leave the case it
	// already fitted alone.
	refWake  = 7
	refSleep = 23

	secondsPerHour = 3600
	hoursPerDay    = 24
	secondsPerDay  = secondsPerHour * hoursPerDay
)

// Log is the activity histogram plus the bookkeeping that keeps it honest.
//
// Day is the epoch day the decay was last applied, so decay happens once a day
// no matter how often canary runs. Hour is the epoch hour last recorded, which
// is what makes a slot mean "I was awake in this hour" instead of "I typed a
// lot" — every call inside the same hour after the first is a no-op.
type Log struct {
	Slots [hoursPerDay]int
	Day   int
	Hour  int
	// Seen identifies the activity that last counted. Claude Code repaints its
	// status row several times a second for as long as it is open — including
	// all night with nobody at the keyboard — and a repaint is not a person.
	// Without this the histogram learns that you are awake at 04:00 because you
	// left a window open, which poisons the one signal the whole estimate rests
	// on.
	Seen int
}

// Load reads the log. A missing file is a first run, not an error, and neither
// is a corrupt one: this sits on the path that draws a prompt, and refusing to
// draw because a counter file has a bad line in it is never the right trade.
//
// Symlinks are refused for the same reason every other canary state file
// refuses them — the path is rewritten constantly, and following a link would
// let anything that can write to ~/.canary redirect that write.
func Load(path string) Log {
	fi, err := os.Lstat(path)
	if err != nil || fi.Mode()&os.ModeSymlink != 0 || !fi.Mode().IsRegular() {
		return Log{}
	}
	f, err := os.Open(path)
	if err != nil {
		return Log{}
	}
	defer f.Close()

	var l Log
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, found := strings.Cut(sc.Text(), "=")
		if !found {
			continue
		}
		switch strings.TrimSpace(k) {
		case "hours":
			l.Slots = parseSlots(v)
		case "day":
			l.Day = atoi(v)
		case "hour":
			l.Hour = atoi(v)
		case "seen":
			l.Seen = atoi(v)
		}
	}
	return l
}

// Save writes the log atomically, in the same key=value shape as the other
// files under ~/.canary so a person can read it with cat and fix it with an
// editor.
func Save(path string, l Log) error {
	parts := make([]string, hoursPerDay)
	for i, n := range l.Slots {
		parts[i] = strconv.Itoa(n)
	}
	var b strings.Builder
	b.WriteString("hours=" + strings.Join(parts, ",") + "\n")
	b.WriteString("day=" + strconv.Itoa(l.Day) + "\n")
	b.WriteString("hour=" + strconv.Itoa(l.Hour) + "\n")
	b.WriteString("seen=" + strconv.Itoa(l.Seen) + "\n")
	return atomicfile.Write(path, []byte(b.String()))
}

// Decayed returns the log aged forward to the given epoch day. It never ages
// backwards: a clock that jumped, a timezone change or a file copied from
// another machine must not resurrect counts.
func Decayed(l Log, day int) Log {
	if l.Day == 0 {
		next := l
		next.Day = day
		return next
	}
	elapsed := day - l.Day
	if elapsed <= 0 {
		return l
	}

	next := Log{Day: day, Hour: l.Hour, Seen: l.Seen}
	if elapsed >= deadDays {
		return next // everything has decayed to nothing; start clean
	}
	next.Slots = l.Slots
	for d := 0; d < elapsed; d++ {
		for i := range next.Slots {
			next.Slots[i] = next.Slots[i] * decayNum / decayDen
		}
	}
	return next
}

// Record notes that you were awake at the given unix time — localHour is the
// hour a clock on the wall would show, which is the only thing the caller knows
// and this package does not. It decays first, so a log opened after a week away
// is aged before it is added to rather than after.
//
// marker identifies the activity that prompted the call, and is what separates
// a person from a repaint. Pass something that only moves when someone does: a
// turn count from a status row that redraws on a timer, or the timestamp itself
// from a shell hook that only fires when a command is typed. A call whose
// marker has not changed keeps the bookkeeping and records nothing.
//
// The second return says whether anything changed, which is what lets the
// caller skip the write on the overwhelming majority of calls: you type many
// commands in an hour, the status row redraws thousands of times, and only the
// first of each hour is news.
func Record(l Log, unix, localHour, marker int) (Log, bool) {
	if unix <= 0 || localHour < 0 || localHour >= hoursPerDay {
		return l, false
	}
	hour := unix / secondsPerHour
	next := Decayed(l, unix/secondsPerDay)
	next.Hour = hour

	// One count per hour: a burst of three hundred commands at 15:00 is one
	// afternoon of being awake at 15:00, and letting it outvote a quiet week
	// would read intensity as if it were schedule. So the marker is only
	// consulted at an hour boundary — the only moment it can change anything —
	// which is also what keeps a busy hour down to a single write.
	if l.Hour != hour && marker != l.Seen {
		next.Seen = marker
		if next.Slots[localHour] < maxSlot {
			next.Slots[localHour] += Weight
		}
	}

	if next == l {
		return l, false
	}
	return next, true
}

// Seed builds a log from an outside count of how many days you were active in
// each hour, scaled onto the same footing as a log canary filled in itself.
//
// It exists so the feature is not useless for its first month. The estimator
// needs weeks of evidence before it will say anything, and macOS has already
// been keeping exactly this — which hours the screen was in use — for the last
// four weeks. Seeding from that is the difference between a bird that adapts
// today and one that adapts in September.
func Seed(counts [hoursPerDay]int, days int) Log {
	var l Log
	if days <= 0 {
		return l
	}
	for h, n := range counts {
		if n < 0 {
			continue
		}
		v := n * SteadyState / days
		if v > maxSlot {
			v = maxSlot
		}
		l.Slots[h] = v
	}
	return l
}

// Total is the evidence the log holds.
func (l Log) Total() int {
	sum := 0
	for _, n := range l.Slots {
		sum += n
	}
	return sum
}

// refCenter is the reference schedule's centre of mass, computed rather than
// typed: change refWake or refSleep and the anchor follows.
var refCenter = mustCenter(block(refWake, refSleep))

// Center returns the circular mean of your activity — the hour your day is
// balanced around — together with R, the concentration of the histogram around
// it. ok is false when the log has not earned an opinion.
//
// A circular mean, and not the longest quiet stretch. Hunting for a run of
// idle hours is the obvious approach and it fails on real data: measured
// against a month of this machine's own activity the longest clean gap was
// four hours, because an irregular sleeper has no single hour they are
// reliably asleep in — every candidate hour has some days in it. The mean uses
// all twenty-four hours at once, so a schedule that wobbles by three hours a
// night still has a stable centre, and R says outright when it does not.
func (l Log) Center() (center, r float64, ok bool) {
	if l.Total() < MinTotal {
		return 0, 0, false
	}
	center, r = centerOf(l.Slots)
	if r < minR {
		return center, r, false
	}
	return center, r, true
}

// Offset is how many hours your body clock runs behind the textbook one, and
// therefore how far fatigue.CircadianExcess's curve should be rotated before it
// is read. ok is false when the log has not earned an opinion, and the caller
// should use the literature curve unrotated.
//
// Positive means later: someone whose day is centred five hours after the
// reference gets +5, which slides the deepest trough from 02:00-04:00 to
// 07:00-09:00 — the hours they are in fact asleep.
func (l Log) Offset() (int, bool) {
	center, _, ok := l.Center()
	if !ok {
		return 0, false
	}
	return normalize(int(math.Round(center - refCenter))), true
}

// centerOf is the circular mean and resultant length of a histogram over the
// 24-hour clock. Ordinary averaging cannot do this: the arithmetic mean of
// 23:00 and 01:00 is noon, when the answer is midnight, and only putting the
// hours back on a circle gets that right.
//
// Each hour becomes a unit vector weighted by its count; the sum's angle is
// the mean hour and its length over the total is R, how tightly the activity
// clusters around it.
func centerOf(slots [hoursPerDay]int) (center, r float64) {
	var x, y float64
	total := 0
	for h, n := range slots {
		angle := 2 * math.Pi * float64(h) / hoursPerDay
		x += float64(n) * math.Cos(angle)
		y += float64(n) * math.Sin(angle)
		total += n
	}
	if total == 0 {
		return 0, 0
	}
	center = math.Atan2(y, x) * hoursPerDay / (2 * math.Pi)
	if center < 0 {
		center += hoursPerDay
	}
	return center, math.Hypot(x, y) / float64(total)
}

// mustCenter is centerOf for the reference block, whose R is never zero. It
// exists so refCenter can be a var initialised from the constants above
// instead of a number typed out by hand and left to drift from them.
func mustCenter(slots [hoursPerDay]int) float64 {
	c, _ := centerOf(slots)
	return c
}

// block is the histogram of someone awake from wake to sleep, one unit per
// waking hour. Used to turn the reference schedule into a reference centre.
func block(wake, sleep int) [hoursPerDay]int {
	var out [hoursPerDay]int
	for h := wake; h != sleep; h = (h + 1) % hoursPerDay {
		out[h] = 1
	}
	return out
}

// Shift rotates a wall-clock hour into the curve's frame.
func Shift(hour, offset int) int {
	return ((hour-offset)%hoursPerDay + hoursPerDay) % hoursPerDay
}

// normalize folds an offset into (-12, 12], so "eleven hours late" is reported
// as thirteen hours early only when that is genuinely the shorter way round.
func normalize(off int) int {
	off %= hoursPerDay
	if off > hoursPerDay/2 {
		off -= hoursPerDay
	}
	if off <= -hoursPerDay/2 {
		off += hoursPerDay
	}
	return off
}

// parseSlots reads the comma-separated counters. A short, long or malformed
// list yields whatever prefix was readable and zeros for the rest: a damaged
// log should cost accuracy, never a drawn bird.
func parseSlots(v string) [hoursPerDay]int {
	var out [hoursPerDay]int
	for i, f := range strings.Split(strings.TrimSpace(v), ",") {
		if i >= hoursPerDay {
			break
		}
		n := atoi(f)
		if n > maxSlot {
			n = maxSlot
		}
		out[i] = n
	}
	return out
}

// atoi accepts a plain non-negative integer and nothing else, matching the
// parser in internal/state: squeezing digits out of arbitrary text is how the
// shell version turned injected escape codes into plausible numbers.
func atoi(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0
		}
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}
