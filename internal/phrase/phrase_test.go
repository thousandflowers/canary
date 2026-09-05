package phrase

import (
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/thousandflowers/canary/internal/fatigue"
)

// scripted returns the values it was given, in order, then repeats the last
// one. Waiting for a 1-in-300 branch to turn up on its own is not a test.
type scripted struct {
	values []int
	i      int
}

func (s *scripted) IntN(n int) int {
	if len(s.values) == 0 {
		return 0
	}
	v := s.values[s.i]
	if s.i < len(s.values)-1 {
		s.i++
	}
	if v >= n {
		v = n - 1
	}
	if v < 0 {
		v = 0
	}
	return v
}

// The roll sequences, named for what they mean.
//
// The dice are rolled conditionally — the ultra die only when a condition
// holds, the rare die only when the band is allowed one — so "the next roll
// after silence" is a different die in different contexts. Each helper says
// which context it is for.
//
// speaksCommon works everywhere: no roll of any die comes up zero.
func speaksCommon() *scripted { return &scripted{values: []int{99, 1, 1, 1}} }

// hits takes the first die offered after the silence gate. Which die that is
// depends on the context, which is the point: in a tired, grinding bird it is
// the uncommon die; in a fresh one it is the rare die; on christmas it is ultra.
func hits() *scripted { return &scripted{values: []int{99, 0}} }

// commonThenRandom fixes the tier dice and lets the shuffle be random, which is
// what a test about pool membership needs: a fully scripted generator deals the
// same permutation every time and only ever sees one line of the pool.
//
// The dice are told apart by their range, so this does not have to know how
// many of them a given context rolls. (A pool whose size happened to equal one
// of those ranges would be fixed too; none of the fixtures is that big.)
type commonThenRandom struct{}

func (commonThenRandom) IntN(n int) int {
	switch n {
	case 100:
		return 99 // past the silence gate
	case UltraOdds, RareOdds, UncommonOdds:
		return 1 // miss every tier above common
	default:
		return rand.IntN(n)
	}
}

func testCorpus() Corpus {
	return Corpus{lang: "en", root: ".", fsys: fstest.MapFS{
		"en/states/dead.txt":         file("the canary is quiet."),
		"en/states/fresh.txt":        file("the air is breathable."),
		"en/states/tired.txt":        file("tired one.", "tired two."),
		"en/states/tired+rising.txt": file("coming back up."),
		"en/states/worn.txt":         file("worn through."),
		"en/states/worn+empty.txt":   file("# nothing but a comment"),
		"en/notes/rising.txt":        file("note: rising."),
		"en/returns/from-break.txt":  file("back from somewhere."),
		"en/time/late.txt":           file("it is late."),
		"en/lore/job.txt":            file("lore: the job."),
		"en/lore/detector.txt":       file("lore: the detector."),
		"en/lore/cage.txt":           file("rare: the cage."),
		"en/lore/facts.txt":          file("rare: a fact."),
		"en/worldly/outside.txt":     file("worldly: outside."),
		"en/worldly/culture.txt":     file("worldly: culture."),
		"en/worldly/subcultures.txt": file("worldly: subcultures."),
		"en/triggers/no-tests.txt":   file("trigger: no tests."),
		"en/triggers/same-file.txt":  file("trigger: same file."),
		"en/ephemeral/2020.txt":      file("ephemeral: two thousand and twenty."),
		"en/ephemeral/2026.txt":      file("ephemeral: this year."),
		"en/ultra/christmas.txt":     file("ultra: the mine is closed."),
		"en/ultra/four-oh-four.txt":  file("ultra: nobody is awake."),
		"mine/untranslated.txt":      file("sisu.", "saudade."),
	}}
}

func file(lines ...string) *fstest.MapFile {
	return &fstest.MapFile{Data: []byte(strings.Join(lines, "\n") + "\n")}
}

func noBag() *Bag { return LoadBag("") }

func TestLinesDropsEverythingThatIsNotAPhrase(t *testing.T) {
	c := Corpus{lang: "en", root: ".", fsys: fstest.MapFS{
		"en/x.txt": file("# a comment", "", "  ", "a real line.", "attributed line. -- @someone", "with an \x1b[31mescape\x1b[0m"),
	}}

	got := c.Lines("en/x.txt")
	want := []string{"a real line.", "attributed line.", "with an [31mescape[0m"}
	if len(got) != len(want) {
		t.Fatalf("got %d lines %q, want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d: got %q, want %q", i, got[i], want[i])
		}
	}
	if strings.ContainsRune(strings.Join(got, ""), 0x1b) {
		t.Error("an escape sequence survived into a phrase")
	}
}

func TestHasIsFalseForAFileWithNoPhrasesInIt(t *testing.T) {
	c := testCorpus()
	if c.Has("en/states/worn+empty.txt") {
		t.Error("a comment-only file counted as a pool")
	}
	if !c.Has("en/states/worn.txt") {
		t.Error("a real file did not count")
	}
}

func TestDeadBypassesTheSilenceRoll(t *testing.T) {
	// Its single line, and the silence that follows it, is the loudest thing
	// this tool says (VOICE.md rule 9).
	silent := &scripted{values: []int{0}} // would silence any other band
	got := Pick(testCorpus(), Context{Band: fatigue.Dead}, silent, noBag(), nil)
	if got.Text != "the canary is quiet." {
		t.Errorf("the dead bird stayed quiet: %+v", got)
	}
}

func TestMostTransitionsSayNothing(t *testing.T) {
	quiet := &scripted{values: []int{SilenceRate - 1}}
	if got := Pick(testCorpus(), Context{Band: fatigue.Tired, Note: "steady"}, quiet, noBag(), nil); got.Text != "" {
		t.Errorf("a roll under the silence rate spoke anyway: %+v", got)
	}
}

func TestCommonTierPrefersStatePlusNote(t *testing.T) {
	got := Pick(testCorpus(), Context{Band: fatigue.Tired, Note: "rising"}, speaksCommon(), noBag(), nil)
	if got.Text != "coming back up." {
		t.Errorf("state+note file was not preferred: %+v", got)
	}
}

func TestWornSaysOneThingAndDoesNotChat(t *testing.T) {
	// worn is the actionable band: no notes, no returns, no lore (VOICE.md §6).
	ctx := Context{Band: fatigue.Worn, Note: "rising", Return: "from-break", OnBreak: true}
	for i := 0; i < 20; i++ {
		got := Pick(testCorpus(), ctx, speaksCommon(), noBag(), nil)
		if got.Text != "" && got.Text != "worn through." {
			t.Fatalf("worn borrowed a line it may not have: %+v", got)
		}
	}
	// worn never rolls for rare, so the first die after silence is uncommon —
	// and an uncommon roll at worn degrades to silence rather than to lore.
	if got := Pick(testCorpus(), ctx, hits(), noBag(), nil); got.Text != "" {
		t.Errorf("worn drew from the lore tier: %+v", got)
	}
}

func TestRareIsGatedOnRecoveringNotOnHours(t *testing.T) {
	// The rule that cannot be traded away: an encounter gated on session length
	// would pay you to keep working, which inverts the whole tool.
	c := testCorpus()

	if got := Pick(c, Context{Band: fatigue.Fresh, Note: "steady"}, hits(), noBag(), nil); got.Tier != "rare" {
		t.Errorf("fresh should be able to draw rare, got %+v", got)
	}
	// A grinding bird is never even offered the rare die: its first roll is the
	// uncommon one, so a zero there cannot buy a rare line.
	grinding := Context{Band: fatigue.Tired, Note: "falling"}
	if got := Pick(c, grinding, hits(), noBag(), nil); got.Tier == "rare" {
		t.Errorf("a tired grinder drew a rare line: %+v", got)
	}
	recovering := Context{Band: fatigue.Tired, Note: "rising"}
	if got := Pick(c, recovering, hits(), noBag(), nil); got.Tier != "rare" {
		t.Errorf("a recovering bird was denied its rare line: %+v", got)
	}
	if got := Pick(c, Context{Band: fatigue.Worn, OnBreak: true}, hits(), noBag(), nil); got.Tier == "rare" {
		t.Errorf("worn drew rare: %+v", got)
	}
}

func TestTiredRareStaysInTheCage(t *testing.T) {
	// A tired bird earns lore, not the outside world.
	got := Pick(testCorpus(), Context{Band: fatigue.Tired, Note: "rising"}, hits(), noBag(), nil)
	if strings.HasPrefix(got.Text, "worldly:") {
		t.Errorf("tired reached outside: %+v", got)
	}
}

func TestATriggerTakesTheUncommonTier(t *testing.T) {
	// A trigger is the most specific thing the bird can say. When one is live it
	// takes the tier rather than sharing it with lore about 1986.
	ctx := Context{Band: fatigue.Tired, Note: "steady", Triggers: []string{"same-file"}}
	got := Pick(testCorpus(), ctx, hits(), noBag(), nil)
	if got.Text != "trigger: same file." {
		t.Errorf("the live trigger did not win the tier: %+v", got)
	}
}

func TestATriggerWithNoCorpusFallsBackToLore(t *testing.T) {
	// Detectors and phrase files are added separately; a detector that fires
	// with no file behind it must not silence the tier.
	ctx := Context{Band: fatigue.Tired, Note: "steady", Triggers: []string{"interrupted"}}
	got := Pick(testCorpus(), ctx, hits(), noBag(), nil)
	if !strings.HasPrefix(got.Text, "lore:") {
		t.Errorf("an unwritten trigger swallowed the uncommon tier: %+v", got)
	}
}

func TestEphemeralIsReadForTwoYearsAndThenNot(t *testing.T) {
	c := testCorpus()
	in2026 := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	if got := c.Ephemeral(in2026); len(got) != 1 || !strings.Contains(got[0], "2026") {
		t.Errorf("this year's file should be readable, got %q", got)
	}
	in2027 := time.Date(2027, 6, 1, 12, 0, 0, 0, time.UTC)
	if got := c.Ephemeral(in2027); len(got) != 1 || !strings.Contains(got[0], "2026") {
		t.Errorf("last year's file should still be readable, got %q", got)
	}
	in2028 := time.Date(2028, 6, 1, 12, 0, 0, 0, time.UTC)
	if got := c.Ephemeral(in2028); len(got) != 0 {
		t.Errorf("a two-year-old file should have expired on its own, got %q", got)
	}
	// And the 2020 file in the fixture has been dead for years without anybody
	// deciding anything.
	if got := c.Ephemeral(in2026); len(got) == 1 && strings.Contains(got[0], "2020") {
		t.Error("an ancient file was still being read")
	}
}

func TestUltraFiresOnlyOnItsOwnCondition(t *testing.T) {
	c := testCorpus()
	christmas := Context{Band: fatigue.Tired, Note: "steady", Now: time.Date(2026, time.December, 25, 15, 0, 0, 0, time.UTC)}
	if got := Pick(c, christmas, hits(), noBag(), nil); got.Tier != "ultra" {
		t.Errorf("christmas did not fire: %+v", got)
	}
	// On an ordinary day the ultra die is never offered, so the same roll buys
	// something else entirely.
	ordinary := Context{Band: fatigue.Tired, Note: "steady", Now: time.Date(2026, time.December, 24, 15, 0, 0, 0, time.UTC)}
	if got := Pick(c, ordinary, hits(), noBag(), nil); got.Tier == "ultra" {
		t.Errorf("an ordinary day fired an ultra line: %+v", got)
	}
	fourOhFour := Context{Band: fatigue.Tired, Note: "steady", Now: time.Date(2026, time.June, 1, 4, 4, 0, 0, time.UTC)}
	if got := Pick(c, fourOhFour, hits(), noBag(), nil); got.Text != "ultra: nobody is awake." {
		t.Errorf("04:04 did not fire: %+v", got)
	}
	// The seventh session of a day is a condition like any other, but this
	// fixture has no file for it: the tier must decline rather than draw blank.
	seventh := Context{Band: fatigue.Tired, Note: "steady", Now: time.Date(2026, time.June, 1, 15, 0, 0, 0, time.UTC), SessionCount: UltraSession}
	if got := Pick(c, seventh, hits(), noBag(), nil); got.Tier == "ultra" {
		t.Errorf("an ultra tier with no file behind it: %+v", got)
	}
}

func TestMineIsRareLowStatesAndOncePerSession(t *testing.T) {
	// The effect works once. As a standing mode it becomes noise, or it gets
	// pasted into a translator, which is the same thing.
	c := testCorpus()

	if !mineAllowed(Context{Band: fatigue.Fresh}) || !mineAllowed(Context{Band: fatigue.Chirpy}) {
		t.Error("the low states are exactly where an untranslated line belongs")
	}
	for _, band := range []fatigue.Band{fatigue.Tired, fatigue.Worn, fatigue.Dead} {
		if mineAllowed(Context{Band: band}) {
			t.Errorf("%s reached for an untranslated line", band)
		}
	}
	if mineAllowed(Context{Band: fatigue.Fresh, MineSeen: true}) {
		t.Error("a second untranslated line in one session")
	}

	// It is in the rare pool, and only there.
	if !hasMine(rarePool(c, Context{Band: fatigue.Fresh})) {
		t.Error("mine/ is not reachable from a fresh bird's rare tier")
	}
	if hasMine(rarePool(c, Context{Band: fatigue.Fresh, MineSeen: true})) {
		t.Error("mine/ stayed in the pool after it was spent")
	}
	if hasMine(uncommonPool(c, Context{Band: fatigue.Fresh})) || hasMine(commonPool(c, Context{Band: fatigue.Fresh, Note: "steady"})) {
		t.Error("mine/ leaked into a tier that is not rare")
	}

	// And a draw that lands on one is reported, so the caller can spend it.
	// Every other rare line is excluded rather than left to a lottery.
	fresh := Context{Band: fatigue.Fresh, Note: "steady"}
	var others []string
	for _, f := range rarePool(c, fresh) {
		if !strings.HasPrefix(f, "mine/") {
			others = append(others, c.Lines(f)...)
		}
	}
	got := Pick(c, fresh, hits(), noBag(), others)
	if !got.Mine {
		t.Errorf("an untranslated line was not reported as one: %+v", got)
	}
}

func hasMine(pool []string) bool {
	for _, f := range pool {
		if strings.HasPrefix(f, "mine/") {
			return true
		}
	}
	return false
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

func TestADeadBirdWithNoLineStaysQuiet(t *testing.T) {
	// A corpus that lost states/dead.txt must not draw an empty phrase.
	bare := Corpus{lang: "en", root: ".", fsys: fstest.MapFS{"en/states/fresh.txt": file("a line.")}}
	if got := Pick(bare, Context{Band: fatigue.Dead}, speaksCommon(), noBag(), nil); got.Text != "" {
		t.Errorf("drew %+v with no dead line in the corpus", got)
	}
}

func TestATemplateThatCanNeverBeFilledIsSilence(t *testing.T) {
	// Rather than a row with a hole in it. Three attempts, then nothing.
	c := Corpus{lang: "en", root: ".", fsys: fstest.MapFS{
		"en/states/tired.txt": file("{file} was rewritten again."),
	}}
	ctx := Context{Band: fatigue.Tired, Note: "steady"}
	if got := Pick(c, ctx, speaksCommon(), noBag(), nil); got.Text != "" {
		t.Errorf("drew %+v from a template nothing can fill", got)
	}
}

func TestATemplateIsFilledWhenTheBirdKnowsTheAnswer(t *testing.T) {
	c := Corpus{lang: "en", root: ".", fsys: fstest.MapFS{
		"en/states/tired.txt": file("{file} again. | that file again."),
	}}
	ctx := Context{Band: fatigue.Tired, Note: "steady", Slots: Slots{"file": "state.go"}}
	if got := Pick(c, ctx, speaksCommon(), noBag(), nil); got.Text != "state.go again." {
		t.Errorf("got %+v", got)
	}
}

func TestTheReturnAndLatePoolsJoinTheCommonTier(t *testing.T) {
	// Coming back from somewhere, and the hour, are both things the bird can
	// remark on — but only in a band that is allowed to chat.
	c := testCorpus()
	ctx := Context{Band: fatigue.Tired, Note: "steady", Return: "from-break", Late: true}

	seen := map[string]bool{}
	for i := 0; i < 60; i++ {
		if got := Pick(c, ctx, commonThenRandom{}, noBag(), nil); got.Text != "" {
			seen[got.Text] = true
		}
	}
	if !seen["back from somewhere."] {
		t.Errorf("the returns pool never came up: %v", keys(seen))
	}
	if !seen["it is late."] {
		t.Errorf("the late pool never came up: %v", keys(seen))
	}
}

func keys(m map[string]bool) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestAllSkipsWhatItCannotWalkInto(t *testing.T) {
	// A corpus with an unreadable directory in it still lints and still draws;
	// the alternative is one bad mode taking the whole bird down.
	if os.Geteuid() == 0 {
		t.Skip("running as root; the mode would be ignored")
	}
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "en", "states"), 0o755)
	os.WriteFile(filepath.Join(dir, "en", "states", "fresh.txt"), []byte("a line.\n"), 0o644)
	locked := filepath.Join(dir, "en", "locked")
	os.MkdirAll(locked, 0o755)
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Skip(err)
	}
	t.Cleanup(func() { os.Chmod(locked, 0o755) })

	if got := FromDir(dir).All(); len(got) != 1 {
		t.Errorf("All returned %q", got)
	}
}

func TestFilesAndAllHandleADirectoryThatIsNotThere(t *testing.T) {
	c := testCorpus()
	if got := c.Files("en/nowhere"); got != nil {
		t.Errorf("Files on a missing directory = %q", got)
	}
	if got := c.All(); len(got) == 0 {
		t.Error("All found nothing in a corpus with files in it")
	}
}
