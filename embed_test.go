package canary

import (
	"testing"
	"time"

	"github.com/thousandflowers/canary/internal/fatigue"
	"github.com/thousandflowers/canary/internal/lint"
	"github.com/thousandflowers/canary/internal/phrase"
)

// The corpus that ships is the corpus that gets linted. Running the same checks
// `canary lint` runs, over the embedded tree, is what makes a phrase PR a
// three-second yes: everything mechanical is answered before review starts.
func TestShippedCorpusPassesTheLinter(t *testing.T) {
	for _, f := range lint.Check(phrase.FromFS(Corpus)) {
		t.Error(f)
	}
}

// alwaysCommon rolls past the silence gate and lands on the common tier, so a
// reachability test does not depend on getting lucky.
type alwaysCommon struct{}

func (alwaysCommon) IntN(n int) int {
	switch {
	case n == 100:
		return 99 // past the silence gate
	case n <= 1:
		return 0 // IntN's contract is [0,n); a one-line pool has one answer
	default:
		return 1 // neither the rare, the uncommon, nor the ultra roll
	}
}

func TestEveryBandCanSpeak(t *testing.T) {
	// A file holding only its header comment is a placeholder — a category
	// somebody opened and has not filled — and that is allowed. A whole BAND
	// that can never say anything is not: it looks exactly like a bug in the
	// phrase engine and never is.
	c := phrase.FromFS(Corpus)
	bag := phrase.LoadBag("")
	for _, band := range []fatigue.Band{fatigue.Fresh, fatigue.Chirpy, fatigue.Tired, fatigue.Worn, fatigue.Dead} {
		for _, note := range []string{"rising", "falling", "steady", "unknown"} {
			ctx := phrase.Context{Band: band, Note: note, Now: time.Now()}
			if phrase.Pick(c, ctx, alwaysCommon{}, bag, nil).Text == "" {
				t.Errorf("%s+%s has no line to draw", band, note)
			}
		}
	}
}

func TestEveryDetectedTriggerHasPhrases(t *testing.T) {
	// The mirror of the linter's check: that one catches a file with no
	// detector, this one catches a detector with no file — a trigger that fires
	// and then has nothing to say, which reads as the phrase engine being
	// broken.
	c := phrase.FromFS(Corpus)
	bag := phrase.LoadBag("")
	for _, trigger := range triggerNames(t) {
		ctx := phrase.Context{
			Band:     fatigue.Tired,
			Note:     "steady",
			Triggers: []string{trigger},
			Now:      time.Now(),
		}
		// Past the silence gate, then the uncommon roll. A tired bird that is
		// not recovering never rolls for rare at all, so the uncommon die is
		// the very next one.
		r := &fixed{values: []int{99, 0}}
		if got := phrase.Pick(c, ctx, r, bag, nil); got.Text == "" || got.Tier != "uncommon" {
			t.Errorf("trigger %q drew %+v, want a line from its own pool", trigger, got)
		}
	}
}

func TestEphemeralLinesAreInDate(t *testing.T) {
	// A year file is read for two years and then stops on its own. This fails
	// when the newest file falls out of that window — which is the reminder to
	// write next year's, not a bug.
	c := phrase.FromFS(Corpus)
	if got := c.Ephemeral(time.Now()); len(got) == 0 {
		t.Error("no ephemeral file is in date: add phrases/en/ephemeral/<this year>.txt")
	}
}

// fixed returns the values it was given, in order, then repeats the last one.
type fixed struct {
	values []int
	i      int
}

func (f *fixed) IntN(n int) int {
	if len(f.values) == 0 {
		return 0
	}
	v := f.values[f.i]
	if f.i < len(f.values)-1 {
		f.i++
	}
	if v >= n {
		v = n - 1
	}
	if v < 0 {
		v = 0
	}
	return v
}

func triggerNames(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, f := range phrase.FromFS(Corpus).Files("en/triggers") {
		out = append(out, trimName(f))
	}
	if len(out) == 0 {
		t.Fatal("no trigger files in the corpus")
	}
	return out
}

func trimName(p string) string {
	if i := lastIndex(p, '/'); i >= 0 {
		p = p[i+1:]
	}
	return p[:len(p)-len(".txt")]
}

func lastIndex(s string, b byte) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == b {
			return i
		}
	}
	return -1
}
