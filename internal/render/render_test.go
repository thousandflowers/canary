package render

import (
	"strings"
	"testing"

	"github.com/thousandflowers/canary/internal/fatigue"
)

func TestPromptDrawsTwoRowsPerBand(t *testing.T) {
	for _, b := range []fatigue.Band{fatigue.Fresh, fatigue.Chirpy, fatigue.Tired, fatigue.Worn} {
		out := Prompt(b, 50, false)
		if n := strings.Count(out, "\n"); n != 2 {
			t.Errorf("%s: %d rows, want 2:\n%s", b, n, out)
		}
	}
}

func TestDeadPromptNamesTheWayOut(t *testing.T) {
	// The only place canary tells you what to do rather than showing you where
	// you are. It earns that by being the band a break is called for.
	out := Prompt(fatigue.Dead, 95, false)
	if !strings.Contains(out, "canary reset") {
		t.Errorf("the dead bird should name its own way out:\n%s", out)
	}
	if !strings.Contains(out, "░ x ▌v") {
		t.Errorf("the dead bird's cage has crumbled on the left:\n%s", out)
	}
}

func TestOnlyChirpySingsUnprompted(t *testing.T) {
	if !strings.Contains(Prompt(fatigue.Chirpy, 35, false), "♪") {
		t.Error("chirpy lost its note")
	}
	for _, b := range []fatigue.Band{fatigue.Fresh, fatigue.Tired, fatigue.Worn, fatigue.Dead} {
		if strings.Contains(Prompt(b, 50, false), "♪") {
			t.Errorf("%s should not sing at the prompt", b)
		}
	}
}

func TestPromptScoreIsOptional(t *testing.T) {
	if strings.Contains(Prompt(fatigue.Tired, 60, false), "[tired 60]") {
		t.Error("the score showed without being asked for")
	}
	if !strings.Contains(Prompt(fatigue.Tired, 60, true), "[tired 60]") {
		t.Error("the score was asked for and did not show")
	}
}

func TestStatuslineHasNoTrailingNewline(t *testing.T) {
	// Claude Code allows one status line command, so canary is appended to
	// whatever else is there; a trailing newline would push the next segment
	// onto a row of its own.
	out := Statusline(Status{Band: fatigue.Tired, StatName: "t", Columns: 80})
	if strings.HasSuffix(out, "\n") {
		t.Errorf("statusline ends with a newline:\n%q", out)
	}
}

func TestStatuslineStatRow(t *testing.T) {
	s := Status{Band: fatigue.Worn, Minutes: 58, Turns: 41, StatName: "t", Errors: 3, Debt: 12, Score: 85, Columns: 80}

	quiet := Statusline(s)
	if !strings.HasPrefix(quiet, " worn · 58m · 41t\n") {
		t.Errorf("stat row: %q", firstLine(quiet))
	}

	s.ShowScore = true
	loud := Statusline(s)
	if !strings.HasPrefix(loud, " worn · 58m · 41t · 3e · d12 · 85\n") {
		t.Errorf("stat row with the numbers: %q", firstLine(loud))
	}
}

func TestSilentBirdShowsTheNoteExceptWhenDead(t *testing.T) {
	// One slot, right of the beak: the note when silent, the phrase when
	// speaking, nothing at all when dead and done talking.
	if !strings.Contains(Statusline(Status{Band: fatigue.Fresh, Columns: 80}), "♪") {
		t.Error("a silent live bird should show its note")
	}
	if strings.Contains(Statusline(Status{Band: fatigue.Dead, Columns: 80}), "♪") {
		t.Error("the dead bird kept humming")
	}
}

func TestPhraseTakesTheNotesSlot(t *testing.T) {
	out := Statusline(Status{Band: fatigue.Tired, Phrase: "sliding. slowly. audibly.", Columns: 80})
	if !strings.Contains(out, "⌐ sliding. slowly. audibly.") {
		t.Errorf("phrase missing from the slot:\n%s", out)
	}
	if strings.Contains(out, "♪") {
		t.Errorf("note and phrase share one slot, both were drawn:\n%s", out)
	}
}

func TestASCIISwapsBothGlyphs(t *testing.T) {
	out := Statusline(Status{Band: fatigue.Tired, Phrase: "a line worth reading", ASCII: true, Columns: 80})
	if strings.ContainsAny(out, "⌐♪") {
		t.Errorf("ASCII mode still drew a UTF-8 glyph:\n%s", out)
	}
	if !strings.Contains(out, "- a line worth reading") {
		t.Errorf("ASCII tail missing:\n%s", out)
	}
}

func TestNightsLineOnlyAfterTwo(t *testing.T) {
	// Escalation is decoupled from the face: the demotion can calm a dead bird
	// to worn, but the count keeps saying what the art no longer shows.
	if strings.Contains(Statusline(Status{Band: fatigue.Worn, Nights: 1, Columns: 80}), "nights past") {
		t.Error("one night is not a streak")
	}
	out := Statusline(Status{Band: fatigue.Worn, Nights: 3, Columns: 80})
	if !strings.HasSuffix(out, "✕ 3 nights past your limit") {
		t.Errorf("streak line missing:\n%s", out)
	}
}

func TestFitTruncatesAtAWordBoundary(t *testing.T) {
	text := "the air is breathable and i am bored out of my small feathered mind"
	got := Fit(text, 50, 0) // 50 - 12 overhead = 38 cells
	if got == "" {
		t.Fatal("dropped a phrase that had room for a stub")
	}
	if len(got) >= len(text) {
		t.Errorf("nothing was truncated: %q", got)
	}
	if strings.HasSuffix(got, " ") || !strings.HasPrefix(text, got) {
		t.Errorf("cut is not a clean prefix at a word boundary: %q", got)
	}
	if strings.Contains(got, "…") {
		t.Error("an ellipsis promises a rest of the sentence that is not coming")
	}
}

func TestFitMeasuresCellsNotCharacters(t *testing.T) {
	// The shell measured with ${#text}, which counts a double-width glyph as
	// one column, so a CJK phrase wrapped the row it was supposed to fit.
	wide := strings.Repeat("宽", 30) + " tail"
	if got := Fit(wide, 40, 0); got != "" && width(got) > 40-12 {
		t.Errorf("fit returned %d cells for a %d-cell budget", width(got), 40-12)
	}
}

func TestFitRefusesWhatIsNotWorthDrawing(t *testing.T) {
	if got := Fit("anything at all", 20, 0); got != "" {
		t.Errorf("a 8-cell budget should draw nothing, got %q", got)
	}
	if got := Fit("supercalifragilisticexpialidocious-and-then-some", 40, 0); got != "" {
		t.Errorf("one unbreakable word has no honest truncation, got %q", got)
	}
	if got := Fit("a short line that fits", 200, 190); got != "" {
		t.Errorf("reserved columns were ignored, got %q", got)
	}
}

func TestFitLeavesShortPhrasesAlone(t *testing.T) {
	const text = "i feel fine. that's never a good sign."
	if got := Fit(text, 120, 0); got != text {
		t.Errorf("a phrase that fits was altered: %q", got)
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// width is the test's own cell counter, so a bug in Fit cannot hide behind the
// same helper Fit uses.
func width(s string) int {
	n := 0
	for _, r := range s {
		switch {
		case r >= 0x1100 && r <= 0x115F, r >= 0x2E80 && r <= 0xA4CF,
			r >= 0xAC00 && r <= 0xD7A3, r >= 0xF900 && r <= 0xFAFF,
			r >= 0xFF00 && r <= 0xFF60, r >= 0xFFE0 && r <= 0xFFE6:
			n += 2
		default:
			n++
		}
	}
	return n
}
