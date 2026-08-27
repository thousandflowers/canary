// Package fatigue holds the score formula the three birds share.
//
// Every number here is a straight port of canary.sh / canary-statusline.sh and
// must stay bit-for-bit identical to them: the shell suites assert that the
// prompt bird and the statusline bird tell the same story, and that parity is
// the property worth protecting, not any single implementation.
//
// All arithmetic is integer arithmetic on purpose. The shell had no choice;
// Go does, and floats would silently drift the bands away from the shell's.
package fatigue

// Band is the bird's state: five labelled anchors of the Karolinska Sleepiness
// Scale. KSS>=7 ("sleepy") is where fatigue-risk protocols call for a break,
// which is why Worn, not Tired, is the band worth acting on.
type Band string

const (
	Fresh  Band = "fresh"  // KSS1
	Chirpy Band = "chirpy" // KSS3
	Tired  Band = "tired"  // KSS5, "neither alert nor sleepy"
	Worn   Band = "worn"   // KSS7, "sleepy" — the actionable line
	Dead   Band = "dead"   // KSS9
)

// Defaults for the env-var knobs. Exported so main can document them in one
// place instead of scattering literals through flag parsing.
const (
	DefaultNightMult = 150 // multiplier at the bottom of the circadian trough
	DefaultErrWeight = 3   // points per failed tool call (frustration)
	DefaultRepWeight = 2   // points per extra repeat of one command (stuck)
	DefaultDebtMax   = 30  // cap on yesterday's fatigue carried into today
	MaxScore         = 100
)

// TimePoints maps active minutes to points. Concave, not linear: the vigilance
// decrement is front-loaded — roughly half of it lands inside the first ~15
// min, reaction times climb reliably past ~30 min, costs steepen after ~60 min,
// and then it flattens rather than continuing straight up.
//
//	15m->7  30m->14  1h->26  2h->43  3h->55  5h->72  8h->86  12h->97
//
// A straight line under-read the first hour (60 min of solid work scored 20,
// still "chirpy") and pinned everything past 5h at 100, which turned the dead
// bird into wallpaper.
func TimePoints(min int) int {
	if min <= 0 {
		return 0
	}
	return min * 130 / (min + 240)
}

// CircadianExcess returns percentage points to add at full amplitude, by hour.
//
// Shape from the circadian literature: the nadir runs 02:00-06:00 and is
// deepest 02:00-04:00; attention bottoms out 04:00-07:00; there is a real
// post-lunch dip 13:00-16:00 that is circadian, not dietary; and 17:00-21:00 is
// the evening "wake maintenance zone", where alertness is genuinely high.
//
// The rule this replaced — a flat x1.3 from 22:00 to 07:00 — penalised you
// hardest during the part of the evening you are sharpest, and treated 23:00
// like 03:00.
func CircadianExcess(hour int) int {
	switch hour {
	case 2, 3, 4:
		return 50 // deepest trough
	case 5, 6:
		return 40
	case 0, 1:
		return 25
	case 7, 13, 14, 15, 23:
		return 15 // tail of the nadir; the post-lunch dip
	case 16, 22:
		return 5
	default:
		return 0 // 08-12 morning peak, 17-21 wake maintenance
	}
}

// ApplyCircadian scales raw by the time of day and caps it at 100.
//
// nightMult is the multiplier at the bottom of the trough (150 = x1.5 at
// 02:00-04:00); it scales the whole curve, so 100 switches the adjustment off.
// A negative excess is ignored rather than applied, matching the shell's
// `[ "$excess" -gt 0 ]` guard — a sub-100 multiplier must not make the bird
// healthier than it earned.
func ApplyCircadian(raw, hour, nightMult int) int {
	excess := CircadianExcess(hour) * (nightMult - 100) / 50
	if excess > 0 {
		raw = raw * (100 + excess) / 100
	}
	return Cap(raw)
}

// Cap clamps a score to the 0-100 range.
func Cap(s int) int {
	if s > MaxScore {
		return MaxScore
	}
	if s < 0 {
		return 0
	}
	return s
}

// ClaudeCodeRaw is the pre-circadian score inside Claude Code, where the
// transcript gives signals the shell has no access to.
//
// There is deliberately no cadence term. Turns-per-hour divided by a tiny
// early-session `min` exploded into noise, and "slower pace = less tired" is
// backwards for a fatigue meter — it made the score non-monotonic in time, so
// the bird got HEALTHIER as minutes passed.
//
// errors and reps are the best-evidenced terms canary has: mental fatigue
// reliably increases error rates AND perseveration — repeating an action that
// is not working — which is exactly what reps counts.
func ClaudeCodeRaw(min, turns, errors, reps, errWeight, repWeight int) int {
	return TimePoints(min) + turns/2 + errors*errWeight + reps*repWeight
}

// ShellRaw is the pre-circadian score from the shell-prompt bird's state file,
// which has neither an error nor a repetition signal and leans on how much you
// are typing instead.
func ShellRaw(min, promptCount, avgLen int) int {
	return TimePoints(min) + promptCount/2 + avgLen/10
}

// BandFor maps a 0-100 score onto the bird.
func BandFor(score int) Band {
	switch {
	case score <= 20:
		return Fresh
	case score <= 45:
		return Chirpy
	case score <= 70:
		return Tired
	case score <= 90:
		return Worn
	default:
		return Dead
	}
}

// Demote calms a dead bird to worn when today is no worse than the person's own
// recent average.
//
// A perma-grinder who is "dead" every night stops seeing it, so a bird that is
// always dead carries no information. But NOT during a streak: the core finding
// on chronic sleep restriction is that deficits accumulate to severe levels
// "without full awareness of the affected individuals" — the person several
// days deep is precisely the one who cannot feel it, and muting the alarm for
// them inverts the point of the bird. The demotion is relief for a single bad
// day, never for an accumulating one.
//
// absolute (CANARY_DEAD_ABSOLUTE=1) restores a fixed >90.
func Demote(b Band, raw, personal, nights int, absolute bool) Band {
	if b == Dead && !absolute && raw <= personal && nights < 2 {
		return Worn
	}
	return b
}
