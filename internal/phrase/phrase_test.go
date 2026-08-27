package phrase

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/thousandflowers/canary/internal/fatigue"
)

// scripted returns the values it was given, in order, then repeats the last
// one. Waiting for a 1-in-40 branch to turn up on its own is not a test.
type scripted struct {
	values []int
	i      int
}

func (s *scripted) IntN(n int) int {
	if len(s.values) == 0 {
		return 0
	}
	v := s.values[min(s.i, len(s.values)-1)]
	s.i++
	if v >= n {
		v = n - 1
	}
	return v
}

// speaks is a roll sequence that gets past the silence gate and lands on the
// common tier: not silent, not rare, not uncommon.
func speaks() *scripted { return &scripted{values: []int{99, 1, 1, 0}} }

func testCorpus() Corpus {
	return Corpus{fsys: fstest.MapFS{
		"en/states/dead.txt":         &fstest.MapFile{Data: []byte("# one line only\n\nthe canary is quiet.\n")},
		"en/states/fresh.txt":        &fstest.MapFile{Data: []byte("the air is breathable.\n")},
		"en/states/tired.txt":        &fstest.MapFile{Data: []byte("tired one.\ntired two.\n")},
		"en/states/tired+rising.txt": &fstest.MapFile{Data: []byte("coming back up.\n")},
		"en/states/worn.txt":         &fstest.MapFile{Data: []byte("worn through.\n")},
		"en/states/worn+empty.txt":   &fstest.MapFile{Data: []byte("# nothing but a comment\n")},
		"en/notes/rising.txt":        &fstest.MapFile{Data: []byte("note: rising.\n")},
		"en/returns/from-break.txt":  &fstest.MapFile{Data: []byte("back from somewhere.\n")},
		"en/time/late.txt":           &fstest.MapFile{Data: []byte("it is late.\n")},
		"en/lore/job.txt":            &fstest.MapFile{Data: []byte("lore: the job.\n")},
		"en/lore/detector.txt":       &fstest.MapFile{Data: []byte("lore: the detector.\n")},
		"en/lore/cage.txt":           &fstest.MapFile{Data: []byte("rare: the cage.\n")},
		"en/lore/facts.txt":          &fstest.MapFile{Data: []byte("rare: a fact.\n")},
		"en/worldly/outside.txt":     &fstest.MapFile{Data: []byte("worldly: outside.\n")},
		"en/worldly/culture.txt":     &fstest.MapFile{Data: []byte("worldly: culture.\n")},
		"en/worldly/subcultures.txt": &fstest.MapFile{Data: []byte("worldly: subcultures.\n")},
	}, root: "en"}
}

func TestLinesDropsEverythingThatIsNotAPhrase(t *testing.T) {
	c := Corpus{fsys: fstest.MapFS{
		"en/x.txt": &fstest.MapFile{Data: []byte(
			"# a comment\n\n  \na real line.\nattributed line. -- @someone\nwith an \x1b[31mescape\x1b[0m\n")},
	}, root: "en"}

	got := c.Lines("x.txt")
	want := []string{"a real line.", "attributed line.", "with an [31mescape[0m"}
	if len(got) != len(want) {
		t.Fatalf("got %d lines %q, want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d: got %q, want %q", i, got[i], want[i])
		}
	}
	// The escape's introducer is gone; what is left is printable text.
	if strings.ContainsRune(strings.Join(got, ""), 0x1b) {
		t.Error("an escape sequence survived into a phrase")
	}
}

func TestHasIsFalseForAFileWithNoPhrasesInIt(t *testing.T) {
	// An empty file used to win a pool and silence the bird.
	c := testCorpus()
	if c.Has("states/worn+empty.txt") {
		t.Error("a comment-only file counted as a pool")
	}
	if !c.Has("states/worn.txt") {
		t.Error("a real file did not count")
	}
}

func TestMissingFilesAreSkippedNotFatal(t *testing.T) {
	if got := testCorpus().Lines("states/nope.txt", "states/fresh.txt"); len(got) != 1 {
		t.Errorf("a missing file should be skipped, got %q", got)
	}
}

func TestDeadBypassesTheSilenceRoll(t *testing.T) {
	// Its single line, and the silence that follows it, is the loudest thing
	// this tool says (VOICE.md rule 9).
	silent := &scripted{values: []int{0}} // would silence any other band
	got := Pick(testCorpus(), Context{Band: fatigue.Dead}, silent, nil)
	if got != "the canary is quiet." {
		t.Errorf("the dead bird stayed quiet: %q", got)
	}
}

func TestMostTransitionsSayNothing(t *testing.T) {
	quiet := &scripted{values: []int{SilenceRate - 1}}
	if got := Pick(testCorpus(), Context{Band: fatigue.Tired, Note: "steady"}, quiet, nil); got != "" {
		t.Errorf("a roll under the silence rate spoke anyway: %q", got)
	}
}

func TestCommonTierPrefersStatePlusNote(t *testing.T) {
	got := Pick(testCorpus(), Context{Band: fatigue.Tired, Note: "rising"}, speaks(), nil)
	if got != "coming back up." {
		t.Errorf("state+note file was not preferred: %q", got)
	}
}

func TestCommonTierFallsBackToThePlainState(t *testing.T) {
	got := Pick(testCorpus(), Context{Band: fatigue.Tired, Note: "steady"}, speaks(), nil)
	if !strings.HasPrefix(got, "tired ") {
		t.Errorf("plain state fallback missed: %q", got)
	}
}

func TestWornSaysOneThingAndDoesNotChat(t *testing.T) {
	// worn is the actionable band: no notes, no returns, no lore (VOICE.md §6).
	ctx := Context{Band: fatigue.Worn, Note: "rising", Return: "from-break", OnBreak: true}
	for i := 0; i < 20; i++ {
		got := Pick(testCorpus(), ctx, speaks(), nil)
		if got != "" && got != "worn through." {
			t.Fatalf("worn borrowed a line it may not have: %q", got)
		}
	}
	// An uncommon roll at worn degrades to silence rather than to lore. worn
	// never rolls for rare at all, so the second value is the uncommon roll.
	uncommon := &scripted{values: []int{99, 0}}
	if got := Pick(testCorpus(), ctx, uncommon, nil); got != "" {
		t.Errorf("worn drew from the lore tier: %q", got)
	}
}

func TestRareIsGatedOnRecoveringNotOnHours(t *testing.T) {
	// The rule that cannot be traded away: an encounter gated on session length
	// would pay you to keep working, which inverts the whole tool.
	rare := func() *scripted { return &scripted{values: []int{99, 0, 0, 0}} }

	if got := Pick(testCorpus(), Context{Band: fatigue.Fresh, Note: "steady"}, rare(), nil); !strings.HasPrefix(got, "rare:") && !strings.HasPrefix(got, "worldly:") {
		t.Errorf("fresh should be able to draw rare, got %q", got)
	}
	// Tired qualifies only while actually recovering.
	grinding := Context{Band: fatigue.Tired, Note: "falling"}
	if got := Pick(testCorpus(), grinding, rare(), nil); strings.HasPrefix(got, "rare:") || strings.HasPrefix(got, "worldly:") {
		t.Errorf("a tired grinder drew a rare line: %q", got)
	}
	recovering := Context{Band: fatigue.Tired, Note: "rising", OnBreak: true}
	if got := Pick(testCorpus(), recovering, rare(), nil); !strings.HasPrefix(got, "rare:") {
		t.Errorf("a recovering bird was denied its rare line: %q", got)
	}
	// Worn never draws rare, however the dice land.
	if got := Pick(testCorpus(), Context{Band: fatigue.Worn, OnBreak: true}, rare(), nil); strings.HasPrefix(got, "rare:") {
		t.Errorf("worn drew rare: %q", got)
	}
}

func TestTiredRareStaysInTheCage(t *testing.T) {
	// A tired bird earns lore, not the outside world.
	rare := &scripted{values: []int{99, 0, 0, 0}}
	got := Pick(testCorpus(), Context{Band: fatigue.Tired, Note: "rising"}, rare, nil)
	if strings.HasPrefix(got, "worldly:") {
		t.Errorf("tired reached outside: %q", got)
	}
}

func TestARepeatGetsOneRedraw(t *testing.T) {
	// Not a shuffle without replacement, and it does not pretend to be: it
	// removes the visible defect, the same line twice running.
	r := &scripted{values: []int{99, 1, 1, 0, 1}} // draws "tired one." then "tired two."
	got := Pick(testCorpus(), Context{Band: fatigue.Tired, Note: "steady"}, r, []string{"tired one."})
	if got == "tired one." {
		t.Error("the line just used came back immediately")
	}
}

func TestLateAddsItsOwnPool(t *testing.T) {
	ctx := Context{Band: fatigue.Fresh, Note: "steady", Late: true}
	r := &scripted{values: []int{99, 1, 1, 1}} // second entry of the pool
	if got := Pick(testCorpus(), ctx, r, nil); got != "it is late." {
		t.Errorf("late pool not added: %q", got)
	}
}

func TestClassifyReadsTheHumansTrendNotTheScores(t *testing.T) {
	// A score going up means the person is going down.
	if got := Classify(fatigue.Tired, 60, 50, true, 0, 12, 30).Note; got != "falling" {
		t.Errorf("rising score should read as the human falling, got %q", got)
	}
	if got := Classify(fatigue.Tired, 40, 50, true, 0, 12, 30).Note; got != "rising" {
		t.Errorf("falling score should read as the human rising, got %q", got)
	}
	if got := Classify(fatigue.Tired, 50, 50, true, 0, 12, 30).Note; got != "steady" {
		t.Errorf("an unmoved score is steady, got %q", got)
	}
	// No previous score is not a trend of zero.
	if got := Classify(fatigue.Tired, 50, 0, false, 0, 12, 30).Note; got != "unknown" {
		t.Errorf("a first refresh has no trend, got %q", got)
	}
}

func TestClassifyGraduatesTheTimeAway(t *testing.T) {
	cases := []struct {
		gap  int
		want string
	}{
		{-1, ""}, {30, ""}, {90, "short"}, {600, "from-break"}, {7200, "long"}, {200000, "days"},
	}
	for _, c := range cases {
		if got := Classify(fatigue.Fresh, 10, 10, true, c.gap, 12, 0).Return; got != c.want {
			t.Errorf("gap %ds: got %q, want %q", c.gap, got, c.want)
		}
	}
	if !Classify(fatigue.Fresh, 10, 10, true, 600, 12, 0).OnBreak {
		t.Error("ten minutes away is a break")
	}
	if Classify(fatigue.Fresh, 10, 10, true, 90, 12, 0).OnBreak {
		t.Error("ninety seconds is not a break")
	}
}

func TestLateIsAnHourOrALongSession(t *testing.T) {
	if !Classify(fatigue.Fresh, 10, 10, true, 0, 3, 0).Late {
		t.Error("03:00 is late")
	}
	if !Classify(fatigue.Fresh, 10, 10, true, 0, 14, 300).Late {
		t.Error("five hours in is late, whatever the clock says")
	}
	if Classify(fatigue.Fresh, 10, 10, true, 0, 14, 30).Late {
		t.Error("half an hour at 14:00 is not late")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
