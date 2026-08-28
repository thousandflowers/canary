package phrase

import (
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"
)

// realRand is the generator the bird actually uses; the bag's job is to make
// even a good generator stop repeating itself.
type realRand struct{}

func (realRand) IntN(n int) int { return rand.IntN(n) }

func TestABagIsExhaustedBeforeItRepeats(t *testing.T) {
	// A plain rand() over these ten lines repeats within about seven draws, and
	// the rarity dies the first time somebody sees the same line twice in an
	// evening.
	lines := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}
	bag := LoadBag("")

	seen := map[string]int{}
	for i := 0; i < len(lines); i++ {
		seen[bag.Draw("pool", lines, realRand{}, nil)]++
	}
	for _, l := range lines {
		if seen[l] != 1 {
			t.Fatalf("after one full pass %q was drawn %d times: %v", l, seen[l], seen)
		}
	}
}

func TestABagReshufflesWhenItRunsOut(t *testing.T) {
	lines := []string{"a", "b", "c"}
	bag := LoadBag("")
	for i := 0; i < len(lines)*3; i++ {
		if got := bag.Draw("pool", lines, realRand{}, nil); got == "" {
			t.Fatalf("draw %d came back empty", i)
		}
	}
}

func TestRecentLinesAreSkippedAcrossAReshuffle(t *testing.T) {
	// Without this the tail of one cycle reappears at the head of the next,
	// which is the most visible failure mode there is.
	lines := []string{"a", "b", "c", "d"}
	bag := LoadBag("")
	recent := []string{"a", "b"}
	for i := 0; i < 12; i++ {
		got := bag.Draw("pool", lines, realRand{}, recent)
		if got == "a" || got == "b" {
			t.Fatalf("draw %d returned %q, which was in the recent queue", i, got)
		}
	}
}

func TestAPoolInsideTheRecentQueueStillSpeaks(t *testing.T) {
	// A one-line pool that has just been used must not silence the bird
	// forever; the recent queue is a preference, not a prohibition.
	lines := []string{"the only line"}
	if got := LoadBag("").Draw("pool", lines, realRand{}, lines); got != "the only line" {
		t.Errorf("got %q, want the line anyway", got)
	}
}

func TestTheShuffleSurvivesTheProcess(t *testing.T) {
	// The status row is redrawn several times a second, each time in a new
	// process. A bag that lived only in memory would reshuffle constantly and
	// be no better than rand().
	path := filepath.Join(t.TempDir(), "bag.json")
	lines := []string{"a", "b", "c", "d", "e", "f"}

	first := LoadBag(path)
	var drawn []string
	for i := 0; i < 3; i++ {
		drawn = append(drawn, first.Draw("pool", lines, realRand{}, nil))
	}
	if err := first.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	second := LoadBag(path)
	for i := 0; i < 3; i++ {
		got := second.Draw("pool", lines, realRand{}, nil)
		for _, before := range drawn {
			if got == before {
				t.Fatalf("%q came back in the same pass after a reload", got)
			}
		}
	}
}

func TestAChangedCorpusReshuffles(t *testing.T) {
	// An upgrade, or a contributor editing phrases/, must not leave the bag
	// handing out positions that no longer mean what they meant.
	path := filepath.Join(t.TempDir(), "bag.json")
	bag := LoadBag(path)
	bag.Draw("pool", []string{"a", "b", "c"}, realRand{}, nil)
	bag.Save()

	grown := []string{"a", "b", "c", "d", "e"}
	reloaded := LoadBag(path)
	seen := map[string]int{}
	for i := 0; i < len(grown); i++ {
		seen[reloaded.Draw("pool", grown, realRand{}, nil)]++
	}
	for _, l := range grown {
		if seen[l] != 1 {
			t.Fatalf("after the pool changed, %q was drawn %d times: %v", l, seen[l], seen)
		}
	}
}

func TestACorruptBagIsAFreshBag(t *testing.T) {
	// Failing to draw would be a far worse outcome than reshuffling once.
	path := filepath.Join(t.TempDir(), "bag.json")
	os.WriteFile(path, []byte("{not json"), 0o644)
	if got := LoadBag(path).Draw("pool", []string{"a"}, realRand{}, nil); got != "a" {
		t.Errorf("a corrupt bag stopped the bird speaking: %q", got)
	}
}

func TestASymlinkedBagIsRefused(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	link := filepath.Join(dir, "bag.json")
	os.WriteFile(target, []byte("{}"), 0o644)
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	bag := LoadBag(link)
	bag.Draw("pool", []string{"a", "b"}, realRand{}, nil)
	if err := bag.Save(); err != nil {
		t.Fatalf("Save should decline quietly, got %v", err)
	}
	if b, _ := os.ReadFile(target); string(b) != "{}" {
		t.Errorf("Save wrote through a symlink: %q", b)
	}
}

func TestSavingIsSkippedWhenNothingWasDrawn(t *testing.T) {
	// Silence is the common case: two thirds of eligible moments draw nothing,
	// and writing the bag on each of those would be several writes a second.
	path := filepath.Join(t.TempDir(), "bag.json")
	if err := LoadBag(path).Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("the bag was written without a single draw")
	}
}
