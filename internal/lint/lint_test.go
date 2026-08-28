package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thousandflowers/canary/internal/phrase"
)

// corpusWith writes a throwaway corpus and lints it. Files are given as
// path → contents, with one phrase per line.
func corpusWith(t *testing.T, files map[string]string) []Finding {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		full := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return Check(phrase.FromDir(dir))
}

// found reports whether any finding mentions the given fragment.
func found(findings []Finding, fragment string) bool {
	for _, f := range findings {
		if strings.Contains(f.String(), fragment) {
			return true
		}
	}
	return false
}

func TestACleanCorpusHasNothingToSay(t *testing.T) {
	got := corpusWith(t, map[string]string{
		"en/states/fresh.txt": "# a header\n\nthe air is breathable. i'm bored.\n",
		"en/states/dead.txt":  "the canary is quiet.\n",
		"en/lore/job.txt":     "i had a job. it ended in 1986.\n",
	})
	for _, f := range got {
		t.Errorf("unexpected: %s", f)
	}
}

func TestTheVoiceRules(t *testing.T) {
	got := corpusWith(t, map[string]string{
		"en/states/fresh.txt": strings.Join([]string{
			"You are starting with a capital.",
			"you should really take a break!",
			"a line with an emoji 🐦 in it.",
			"four hours and 12 minutes so far.",
			"",
		}, "\n"),
	})

	for _, want := range []string{
		"starts with a capital",
		`contains "should"`,
		`contains "take a break"`,
		`contains "!"`,
		"contains an emoji",
		"prints a number about the session",
	} {
		if !found(got, want) {
			t.Errorf("rule not enforced: %s\ngot: %v", want, got)
		}
	}
}

func TestAYearInLoreIsAFactNotAMeasurement(t *testing.T) {
	// Rule 3 is about the bird printing what it measured about you. A date in a
	// piece of lore is a fact about the world.
	got := corpusWith(t, map[string]string{
		"en/lore/facts.txt": "canaries were used in mines until 1986.\n",
	})
	if found(got, "prints a number") {
		t.Errorf("a year in lore was rejected: %v", got)
	}
}

func TestLoreDoesNotLookAtYou(t *testing.T) {
	got := corpusWith(t, map[string]string{
		"en/lore/cage.txt": "some canaries are pets. you would like them.\n",
	})
	if !found(got, "lore addresses the reader") {
		t.Errorf("lore was allowed to address the reader: %v", got)
	}
}

func TestRightToLeftIsRefusedEverywhere(t *testing.T) {
	// Arabic and Hebrew in a status row shared with another segment produce
	// bidi corruption nobody can control — mine/ included.
	got := corpusWith(t, map[string]string{
		"mine/untranslated.txt": "sisu. sitä ei käännetä.\nדבר אחד.\n",
	})
	if !found(got, "right-to-left") {
		t.Errorf("a right-to-left line was accepted: %v", got)
	}
}

func TestDeadIsPinnedAtOneLine(t *testing.T) {
	got := corpusWith(t, map[string]string{
		"en/states/dead.txt": "the canary is quiet.\nand one more thing.\n",
	})
	if !found(got, "pinned at exactly one line") {
		t.Errorf("a second dead line was accepted: %v", got)
	}
}

func TestFilesNothingWillEverReadAreReported(t *testing.T) {
	got := corpusWith(t, map[string]string{
		"en/states/exhausted.txt":     "a band that does not exist.\n",
		"en/states/worn+sideways.txt": "a note that does not exist.\n",
		"en/notes/sideways.txt":       "still not a note.\n",
		"en/triggers/invented.txt":    "a trigger with no detector.\n",
		"en/ephemeral/soon.txt":       "not a year.\n",
	})
	for _, want := range []string{
		"is not a band",
		"is not a note",
		"no detector for this trigger",
		"ephemeral files are named for a year",
	} {
		if !found(got, want) {
			t.Errorf("not reported: %s\ngot: %v", want, got)
		}
	}
}

func TestTemplatesMustHaveAWayOut(t *testing.T) {
	got := corpusWith(t, map[string]string{
		"en/states/fresh.txt": strings.Join([]string{
			"{repo} has been open since {time}. | this has been open a while.",
			"{file} has been rewritten {n} times tonight.",
			"{nonsense} is not a slot anything can fill.",
			"",
		}, "\n"),
	})
	if found(got, "since {time}") {
		t.Errorf("a template with a fallback was rejected: %v", got)
	}
	if !found(got, "template with no fallback") {
		t.Errorf("a template with no way out was accepted: %v", got)
	}
	if !found(got, "unknown slot {nonsense}") {
		t.Errorf("an unfillable slot was accepted: %v", got)
	}
}

func TestDuplicatesAndNearDuplicates(t *testing.T) {
	got := corpusWith(t, map[string]string{
		"en/states/fresh.txt":  "the air is breathable. i'm bored.\n",
		"en/states/chirpy.txt": "the air is breathable. i'm bored.\n",
		"en/states/tired.txt":  "the air is breathable, i am bored.\n",
	})
	if !found(got, "already in") {
		t.Errorf("an exact duplicate was accepted: %v", got)
	}
	if !found(got, "nearly the same as") {
		t.Errorf("a retyped line was accepted: %v", got)
	}
}

func TestTwoShortLinesThatMerelyRhymeAreLeftAlone(t *testing.T) {
	// Three edits is most of a short phrase; flagging those would make the
	// linter something contributors argue with instead of trust.
	got := corpusWith(t, map[string]string{
		"en/states/fresh.txt": "still here.\nstill dying.\n",
	})
	if found(got, "nearly the same as") {
		t.Errorf("two short distinct lines were called duplicates: %v", got)
	}
}

func TestWidthIsMeasuredInCells(t *testing.T) {
	got := corpusWith(t, map[string]string{
		"en/states/fresh.txt": strings.Repeat("宽", MaxCells/2+1) + "\n",
	})
	if !found(got, "over the") {
		t.Errorf("a double-width line under the character count was accepted: %v", got)
	}
}

func TestFindingsAreStable(t *testing.T) {
	// The output is read by a person twice: once in CI, once locally. It has to
	// come back the same both times.
	files := map[string]string{
		"en/states/fresh.txt":  "You have a capital.\n",
		"en/states/chirpy.txt": "another! line.\n",
	}
	first := corpusWith(t, files)
	second := corpusWith(t, files)
	if len(first) != len(second) {
		t.Fatalf("%d findings then %d", len(first), len(second))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Errorf("finding %d differs between runs:\n%s\n%s", i, first[i], second[i])
		}
	}
}
