package fatigue

import (
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

// The shell implementation is the specification. These tests do not assert
// against numbers typed out by hand — hand-typed expectations only prove the
// port matches what the porter believed, which is exactly the thing in doubt.
// They run the real shell arithmetic and diff it against the Go.
//
// One bash process emits the whole sweep; invoking bash per case turned a
// sub-second test into a minute of fork overhead.
func shellSweep(t *testing.T, script string) []int {
	t.Helper()
	out, err := exec.Command("bash", "-c", script).Output()
	if err != nil {
		t.Fatalf("shell oracle failed: %v", err)
	}
	fields := strings.Fields(string(out))
	got := make([]int, 0, len(fields))
	for _, f := range fields {
		n, err := strconv.Atoi(f)
		if err != nil {
			t.Fatalf("oracle emitted non-integer %q: %v", f, err)
		}
		got = append(got, n)
	}
	return got
}

// TimePoints is the term that dominates the score, and integer division is
// where a port silently drifts: Go and the shell must truncate the same way at
// every single minute, not merely at the eight documented anchors.
func TestTimePointsMatchesShell(t *testing.T) {
	const maxMin = 1500 // past 24h of active time; well beyond any real session
	want := shellSweep(t, `for m in $(seq 0 1500); do echo $(( m * 130 / (m + 240) )); done`)
	if len(want) != maxMin+1 {
		t.Fatalf("oracle returned %d values, want %d", len(want), maxMin+1)
	}
	for m := 0; m <= maxMin; m++ {
		if got := TimePoints(m); got != want[m] {
			t.Errorf("TimePoints(%d) = %d, shell says %d", m, got, want[m])
		}
	}
}

// The documented anchors from the comment block in canary.sh. If the curve is
// ever retuned these are the numbers the README promises a reader.
func TestTimePointsAnchors(t *testing.T) {
	for _, c := range []struct{ min, want int }{
		{15, 7}, {30, 14}, {60, 26}, {120, 43},
		{180, 55}, {300, 72}, {480, 86}, {720, 97},
	} {
		if got := TimePoints(c.min); got != c.want {
			t.Errorf("TimePoints(%d) = %d, documented as %d", c.min, got, c.want)
		}
	}
}

func TestCircadianExcessMatchesShell(t *testing.T) {
	want := shellSweep(t, `
		excess() { case $1 in
			2|3|4) echo 50 ;; 5|6) echo 40 ;; 0|1) echo 25 ;;
			7|13|14|15|23) echo 15 ;; 16|22) echo 5 ;; *) echo 0 ;;
		esac; }
		for h in $(seq 0 23); do excess $h; done`)
	for h := 0; h <= 23; h++ {
		if got := CircadianExcess(h); got != want[h] {
			t.Errorf("CircadianExcess(%d) = %d, shell says %d", h, got, want[h])
		}
	}
}

// The multiplier scales the whole curve, so an off-by-one in the rounding shows
// up only at some (raw, hour, mult) combinations — hence the full cross product
// rather than a few spot checks.
func TestApplyCircadianMatchesShell(t *testing.T) {
	want := shellSweep(t, `
		excess() { case $1 in
			2|3|4) echo 50 ;; 5|6) echo 40 ;; 0|1) echo 25 ;;
			7|13|14|15|23) echo 15 ;; 16|22) echo 5 ;; *) echo 0 ;;
		esac; }
		for raw in 0 1 7 13 26 43 55 72 86 97 100; do
		  for h in $(seq 0 23); do
		    for nm in 100 120 150 200; do
		      r=$raw
		      e=$(( $(excess $h) * (nm - 100) / 50 ))
		      [ "$e" -gt 0 ] && r=$(( r * (100 + e) / 100 ))
		      [ "$r" -gt 100 ] && r=100
		      echo "$r"
		    done
		  done
		done`)
	i := 0
	for _, raw := range []int{0, 1, 7, 13, 26, 43, 55, 72, 86, 97, 100} {
		for h := 0; h <= 23; h++ {
			for _, nm := range []int{100, 120, 150, 200} {
				if got := ApplyCircadian(raw, h, nm); got != want[i] {
					t.Errorf("ApplyCircadian(raw=%d, hour=%d, mult=%d) = %d, shell says %d",
						raw, h, nm, got, want[i])
				}
				i++
			}
		}
	}
}

// A multiplier below 100 must not be able to make the bird look healthier than
// it earned: the shell guards the scaling behind `[ "$excess" -gt 0 ]`, and
// dropping that guard in the port would quietly hand users a cheat code.
func TestSubHundredMultiplierNeverDiscounts(t *testing.T) {
	for h := 0; h <= 23; h++ {
		if got := ApplyCircadian(80, h, 50); got != 80 {
			t.Errorf("ApplyCircadian(80, hour=%d, mult=50) = %d, want 80 (no discount)", h, got)
		}
	}
}

func TestBandBoundaries(t *testing.T) {
	// The exact edges, where an off-by-one changes what the user is told.
	for _, c := range []struct {
		score int
		want  Band
	}{
		{0, Fresh}, {20, Fresh}, {21, Chirpy}, {45, Chirpy},
		{46, Tired}, {70, Tired}, {71, Worn}, {90, Worn}, {91, Dead}, {100, Dead},
	} {
		if got := BandFor(c.score); got != c.want {
			t.Errorf("BandFor(%d) = %q, want %q", c.score, got, c.want)
		}
	}
}

// worn starts at 71, and CANARY_MIN_SCORE=71 is the documented "only once it
// matters" setting. If the band edge ever moves, that advice goes stale.
func TestWornStartsAtSeventyOne(t *testing.T) {
	if BandFor(70) == Worn || BandFor(71) != Worn {
		t.Fatal("worn must begin at 71: README and the brew caveats both say so")
	}
}

func TestDemote(t *testing.T) {
	// Relief for a single bad day...
	if got := Demote(Dead, 80, 90, 0, false); got != Worn {
		t.Errorf("a dead day no worse than the personal average should calm to worn, got %q", got)
	}
	// ...but never for an accumulating one.
	if got := Demote(Dead, 80, 90, 2, false); got != Dead {
		t.Errorf("two nights deep must stay dead, got %q", got)
	}
	// Worse than your own average is a real signal, not habituation.
	if got := Demote(Dead, 95, 90, 0, false); got != Dead {
		t.Errorf("a day worse than the personal average must stay dead, got %q", got)
	}
	if got := Demote(Dead, 80, 90, 0, true); got != Dead {
		t.Errorf("CANARY_DEAD_ABSOLUTE must restore a fixed >90, got %q", got)
	}
	// Demotion applies to dead only; no other band is touched.
	if got := Demote(Tired, 0, 100, 0, false); got != Tired {
		t.Errorf("Demote must leave non-dead bands alone, got %q", got)
	}
}

func TestCapClampsBothEnds(t *testing.T) {
	// The floor exists because a negative multiplier or a hand-edited history
	// must not produce a bird healthier than fresh.
	if got := Cap(-5); got != 0 {
		t.Errorf("Cap(-5) = %d, want 0", got)
	}
	if got := Cap(MaxScore + 1); got != MaxScore {
		t.Errorf("Cap(101) = %d, want %d", got, MaxScore)
	}
	if got := Cap(50); got != 50 {
		t.Errorf("Cap(50) = %d", got)
	}
}

func TestTheTwoRawFormulas(t *testing.T) {
	// Both are straight ports; the point of asserting them is that the terms
	// stay in the right places.
	if got := ClaudeCodeRaw(60, 10, 2, 3, 3, 2); got != TimePoints(60)+5+6+6 {
		t.Errorf("ClaudeCodeRaw = %d", got)
	}
	if got := ShellRaw(60, 10, 20); got != TimePoints(60)+5+2 {
		t.Errorf("ShellRaw = %d", got)
	}
}
