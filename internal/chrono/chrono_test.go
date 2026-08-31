package chrono

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// steady is what a slot converges to when you are active in that hour every
// day: Weight/(1-15/16), before integer truncation shaves it.
const steady = 256

// awakeAt builds a saturated log for someone active in exactly these hours.
func awakeAt(hours ...int) Log {
	var l Log
	for _, h := range hours {
		l.Slots[h] = steady
	}
	return l
}

func hoursFrom(start, n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = (start + i) % 24
	}
	return out
}

// The reference case. fatigue.CircadianExcess is written around a person who
// gets up at seven, so that person's curve must come back unrotated — a
// calibration that moves the case it already fitted is a regression, not a
// feature.
func TestOffsetLeavesTheTextbookSleeperAlone(t *testing.T) {
	l := awakeAt(hoursFrom(7, 16)...) // awake 07:00-22:59, asleep 23:00-06:59

	off, ok := l.Offset()
	if !ok {
		t.Fatalf("no offset from a full week's worth of a plain schedule")
	}
	if off != 0 {
		t.Errorf("textbook sleeper rotated by %d hours, want 0", off)
	}
}

func TestOffsetFollowsALateSleeper(t *testing.T) {
	l := awakeAt(hoursFrom(12, 16)...) // up at noon, asleep 04:00-11:59

	off, ok := l.Offset()
	if !ok {
		t.Fatalf("no offset for a late sleeper")
	}
	if off != 5 {
		t.Errorf("offset %d, want 5 (noon riser runs five hours behind 07:00)", off)
	}
	// The point of the rotation: the deepest trough must land while they sleep.
	if got := Shift(8, off); got != 3 {
		t.Errorf("08:00 maps to curve hour %d, want 3 (the trough)", got)
	}
}

func TestOffsetFollowsAnEarlyRiser(t *testing.T) {
	l := awakeAt(hoursFrom(4, 16)...) // up at 04:00, asleep 20:00-03:59

	off, ok := l.Offset()
	if !ok {
		t.Fatalf("no offset for an early riser")
	}
	if off != -3 {
		t.Errorf("offset %d, want -3", off)
	}
}

// The arithmetic mean of 23:00 and 01:00 is noon, which is the exact opposite
// of the answer. A night owl straddling midnight is the case that catches it.
func TestCenterWrapsMidnight(t *testing.T) {
	l := awakeAt(22, 23, 0, 1, 2)

	center, _, ok := l.Center()
	if !ok {
		t.Fatal("no centre found")
	}
	if center < 23.5 && center > 0.5 {
		t.Errorf("centre %.2f, want near midnight", center)
	}
}

// The regression that chose this estimator. These are a real month of activity
// off the machine canary was built on, one count per day-hour observed. Its
// longest quiet stretch is four hours, because an irregular sleeper has no
// hour they are reliably asleep in — a run-length estimator refuses to answer
// here, which is the wrong answer for a person who is plainly a night owl.
func TestOffsetReadsAnIrregularNightOwl(t *testing.T) {
	observed := [24]int{17, 18, 16, 11, 8, 5, 0, 0, 1, 1, 8, 5, 10, 13, 17, 18, 18, 11, 9, 9, 8, 13, 13, 15}
	var l Log
	for h, n := range observed {
		l.Slots[h] = n * steady / 30 // a month of days, scaled to the live log
	}

	off, ok := l.Offset()
	if !ok {
		t.Fatal("refused a month of real data")
	}
	if off != 5 {
		t.Errorf("offset %+d, want +5", off)
	}
	// What the rotation buys: 02:00, when this person is demonstrably working
	// well, stops being scored as the bottom of the circadian trough.
	if Shift(2, off) == 3 {
		t.Error("02:00 still lands on the deepest trough")
	}
}

// A schedule with no shape must not produce a confident rotation. R is what
// catches it, and it is the guard the whole estimator rests on.
func TestOffsetRefusesAShapelessSchedule(t *testing.T) {
	l := awakeAt(hoursFrom(0, 24)...)

	if _, r, ok := l.Center(); ok || r > 0.01 {
		t.Errorf("flat histogram: r=%.3f ok=%v, want ~0 false", r, ok)
	}
	if off, ok := l.Offset(); ok || off != 0 {
		t.Errorf("offset %d ok=%v, want 0 false", off, ok)
	}
}

// One late night is not a chronotype. The centre must barely move.
func TestOneLateNightBarelyMovesTheCentre(t *testing.T) {
	l := awakeAt(hoursFrom(7, 16)...)
	before, _, _ := l.Center()

	l.Slots[3] = Weight // awake at 03:00 once
	after, _, ok := l.Center()
	if !ok {
		t.Fatal("lost the centre")
	}
	if d := after - before; d < -0.5 || d > 0.5 {
		t.Errorf("centre moved %.2f hours on one late night", d)
	}
}

func TestRecordCountsAnHourOnlyOnce(t *testing.T) {
	const day = 20000
	base := day*24*3600 + 15*3600 // 15:00 UTC on that day

	l, changed := Record(Log{}, base, 15, base)
	if !changed || l.Slots[15] != Weight {
		t.Fatalf("first record: slot=%d changed=%v", l.Slots[15], changed)
	}

	// Nine more commands in the same hour, each its own activity.
	for i := 1; i <= 9; i++ {
		next, changed := Record(l, base+i*60, 15, base+i*60)
		if changed {
			t.Fatalf("command %d in the same hour asked for a write", i)
		}
		l = next
	}
	if l.Slots[15] != Weight {
		t.Errorf("slot %d after ten commands in one hour, want %d", l.Slots[15], Weight)
	}

	// The next hour is news again.
	l, changed = Record(l, base+3600, 16, base+3600)
	if !changed || l.Slots[16] != Weight {
		t.Errorf("next hour: slot=%d changed=%v", l.Slots[16], changed)
	}
}

// The case this marker exists for. Claude Code redraws its status row for as
// long as it is open, so a laptop left running overnight would otherwise teach
// the histogram that its owner is awake at 03:00 — and that histogram is the
// one thing the whole estimate rests on.
func TestRepaintsAllNightRecordNothing(t *testing.T) {
	const day = 20000
	evening := day*24*3600 + 22*3600

	// One real turn at 22:00, then the window is left open.
	l, _ := Record(Log{}, evening, 22, 7)

	for h := 1; h <= 9; h++ { // 23:00 through 07:00, thousands of repaints
		for i := 0; i < 500; i++ {
			l, _ = Record(l, evening+h*3600+i, (22+h)%24, 7) // same turn count
		}
	}

	if l.Slots[22] == 0 {
		t.Error("the real turn was lost")
	}
	for h := 23; h != 8; h = (h + 1) % 24 {
		if l.Slots[h] != 0 {
			t.Errorf("a repaint counted as being awake at %02d:00 (slot %d)", h, l.Slots[h])
		}
	}

	// And it must wake up properly when a person comes back.
	l, changed := Record(l, evening+10*3600, 8, 8)
	if !changed || l.Slots[8] != Weight {
		t.Errorf("a real turn after the night: slot=%d changed=%v", l.Slots[8], changed)
	}
}

func TestRecordRejectsNonsense(t *testing.T) {
	for _, tc := range []struct{ unix, hour int }{{0, 3}, {-1, 3}, {1 << 30, -1}, {1 << 30, 24}} {
		if _, changed := Record(Log{}, tc.unix, tc.hour, 1); changed {
			t.Errorf("Record(%d, %d) accepted", tc.unix, tc.hour)
		}
	}
}

// The decay is the whole adaptation mechanism: it is what makes the estimate
// follow a schedule that shifts with the season instead of averaging a life.
func TestDecayHalvesInAboutElevenDays(t *testing.T) {
	l := Log{Day: 100}
	l.Slots[9] = steady

	got := Decayed(l, 111).Slots[9]
	if got < 100 || got > 150 {
		t.Errorf("after 11 days: %d, want roughly half of %d", got, steady)
	}
}

func TestDecayNeverRunsBackwards(t *testing.T) {
	l := Log{Day: 100}
	l.Slots[9] = steady

	if got := Decayed(l, 90).Slots[9]; got != steady {
		t.Errorf("a backwards clock changed the log: %d, want %d", got, steady)
	}
}

func TestDecayForgetsALongAbsence(t *testing.T) {
	l := Log{Day: 100}
	l.Slots[9] = steady

	if got := Decayed(l, 100+deadDays+1); got.Total() != 0 {
		t.Errorf("total %d after a year away, want 0", got.Total())
	}
}

// An always-active hour must not drift up or down forever; the histogram is
// only meaningful if its scale is stable.
func TestSteadyStateIsStable(t *testing.T) {
	l := Log{Day: 0}
	for d := 1; d <= 200; d++ {
		l = Decayed(l, d)
		l.Slots[9] += Weight
	}
	if l.Slots[9] < 200 || l.Slots[9] > 300 {
		t.Errorf("steady state %d, want near %d", l.Slots[9], steady)
	}
}

func TestShiftWrapsBothWays(t *testing.T) {
	for _, tc := range []struct{ hour, off, want int }{
		{8, 5, 3}, {2, 5, 21}, {3, -3, 6}, {0, 0, 0}, {23, -1, 0},
	} {
		if got := Shift(tc.hour, tc.off); got != tc.want {
			t.Errorf("Shift(%d, %d) = %d, want %d", tc.hour, tc.off, got, tc.want)
		}
	}
}

// A seeded log has to clear the evidence bar on its own, or the bootstrap has
// bought nothing: the whole point is not waiting a month.
func TestSeedIsImmediatelyUsable(t *testing.T) {
	observed := [24]int{17, 18, 16, 11, 8, 5, 0, 0, 1, 1, 8, 5, 10, 13, 17, 18, 18, 11, 9, 9, 8, 13, 13, 15}

	l := Seed(observed, 30)
	if l.Total() < MinTotal {
		t.Errorf("seeded total %d, below the %d needed to say anything", l.Total(), MinTotal)
	}
	off, ok := l.Offset()
	if !ok || off != 5 {
		t.Errorf("seeded offset %+d ok=%v, want +5 true", off, ok)
	}
}

func TestSeedRejectsNonsense(t *testing.T) {
	full := [24]int{}
	for h := range full {
		full[h] = 30
	}
	if got := Seed(full, 0).Total(); got != 0 {
		t.Errorf("zero days seeded %d", got)
	}
	if got := Seed(full, -5).Total(); got != 0 {
		t.Errorf("negative days seeded %d", got)
	}
	// An hour you were active every day is exactly the steady state.
	if got := Seed(full, 30).Slots[3]; got != SteadyState {
		t.Errorf("every-day hour seeded to %d, want %d", got, SteadyState)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chrono")
	want := awakeAt(hoursFrom(9, 12)...)
	want.Day, want.Hour, want.Seen = 20000, 480000, 42

	if err := Save(path, want); err != nil {
		t.Fatalf("save: %v", err)
	}
	if got := Load(path); got != want {
		t.Errorf("round trip lost data:\n got %+v\nwant %+v", got, want)
	}
}

func TestLoadSurvivesGarbage(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		"empty":     "",
		"junk":      "not a log at all\n",
		"short":     "hours=1,2,3\nday=5\n",
		"escapes":   "hours=\x1b[31m9,2\nday=\x1b[0m7\n",
		"overlong":  "hours=" + "1," + "9,"[0:2] + "999999999999999999999,3\n",
		"negatives": "hours=-5,-6\nday=-3\n",
	} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		l := Load(path) // must not panic, must not invent evidence
		if _, ok := l.Offset(); ok {
			t.Errorf("%s: garbage produced a confident offset", name)
		}
	}
}

func TestLoadRefusesASymlink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real")
	if err := os.WriteFile(real, []byte("hours=99\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if got := Load(link); got != (Log{}) {
		t.Errorf("followed a symlink: %+v", got)
	}
}

func TestLoadMissingFileIsAFreshLog(t *testing.T) {
	if got := Load(filepath.Join(t.TempDir(), "nope")); got != (Log{}) {
		t.Errorf("missing file gave %+v, want zero", got)
	}
}

// The helpers below are only reachable with input a person had to type by hand
// or a file that got damaged, which is exactly why they are worth pinning: they
// are what stands between a bad line in ~/.canary and a bird that will not draw.

func TestNormalizeTakesTheShorterWayRound(t *testing.T) {
	for _, tc := range []struct{ in, want int }{
		{0, 0}, {5, 5}, {-5, -5}, {12, 12}, {13, -11}, {-12, 12}, {-13, 11}, {23, -1}, {25, 1},
	} {
		if got := normalize(tc.in); got != tc.want {
			t.Errorf("normalize(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestParseSlotsBoundsWhatItReads(t *testing.T) {
	// More columns than there are hours: the extras are dropped, not wrapped
	// round onto the morning.
	long := make([]string, 40)
	for i := range long {
		long[i] = "1"
	}
	got := parseSlots(strings.Join(long, ","))
	if got[0] != 1 || got[23] != 1 {
		t.Errorf("lost the first 24 values: %v", got)
	}

	// A number a hand-edit could produce, far past anything the decay can reach.
	if got := parseSlots("99999999"); got[0] != maxSlot {
		t.Errorf("unclamped slot %d, want %d", got[0], maxSlot)
	}
}

func TestAtoiRefusesAnythingButAPlainNumber(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want int
	}{
		{"12", 12}, {"  12  ", 12}, {"", 0}, {"-3", 0}, {"1.5", 0}, {"9e9", 0},
		{"\x1b[31m9", 0},
		{"99999999999999999999999", 0}, // all digits, still not an int
	} {
		if got := atoi(tc.in); got != tc.want {
			t.Errorf("atoi(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestSeedBoundsWhatItIsGiven(t *testing.T) {
	var counts [24]int
	counts[3] = -50     // a negative count is not evidence of anything
	counts[4] = 1 << 20 // nor is an impossible one
	counts[5] = 10
	l := Seed(counts, 10)

	if l.Slots[3] != 0 {
		t.Errorf("negative count seeded %d", l.Slots[3])
	}
	if l.Slots[4] != maxSlot {
		t.Errorf("unclamped seed %d, want %d", l.Slots[4], maxSlot)
	}
	if l.Slots[5] != SteadyState {
		t.Errorf("every-day hour seeded to %d, want %d", l.Slots[5], SteadyState)
	}
}

func TestCenterOfAnEmptyHistogram(t *testing.T) {
	center, r := centerOf([hoursPerDay]int{})
	if center != 0 || r != 0 {
		t.Errorf("centre %.2f r %.2f from nothing, want 0 0", center, r)
	}
}

func TestLoadSurvivesAFileItCannotOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chrono")
	if err := os.WriteFile(path, []byte("hours=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	if os.Geteuid() == 0 {
		t.Skip("root reads anything")
	}

	if got := Load(path); got != (Log{}) {
		t.Errorf("an unreadable log gave %+v, want zero", got)
	}
}
