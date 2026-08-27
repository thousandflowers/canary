package canary

import (
	"io/fs"
	"path"
	"strings"
	"testing"
	"unicode"

	"github.com/mattn/go-runewidth"
	"github.com/thousandflowers/canary/internal/fatigue"
	"github.com/thousandflowers/canary/internal/phrase"
)

// This is VOICE.md §6's linter, run as a test rather than as a subcommand.
//
// It exists so a phrase PR can be a three-second yes instead of an argument
// about tone: everything objective is checked here, and review is left with the
// only question that needs a person — is the line any good.
//
// MaxCells is the width budget. The bird's row costs 12 cells before the phrase
// starts, so this leaves a line readable on a 90-column terminal without the
// truncation in render.Fit ever having to cut it.
const MaxCells = 78

// banned are the phrasings that turn an observation into nagging (VOICE.md
// rules 4 and 5). The bird notices; it does not instruct.
var banned = []string{"should", "must", "you need", "take a break", "remember to", "!"}

func corpusFiles(t *testing.T) []string {
	t.Helper()
	var out []string
	err := fs.WalkDir(Corpus, "phrases", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(p, ".txt") {
			out = append(out, strings.TrimPrefix(p, "phrases/en/"))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the embedded corpus: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("the binary shipped with no corpus at all")
	}
	return out
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
		return 1 // neither the rare nor the uncommon roll
	}
}

func TestEveryBandCanSpeak(t *testing.T) {
	// A file holding only its header comment is a placeholder — a category
	// somebody opened and has not filled — and that is allowed. A whole BAND
	// that can never say anything is not: it looks exactly like a bug in the
	// phrase engine and never is.
	c := phrase.FromFS(Corpus)
	for _, band := range []fatigue.Band{fatigue.Fresh, fatigue.Chirpy, fatigue.Tired, fatigue.Worn, fatigue.Dead} {
		spoke := false
		for _, note := range []string{"rising", "falling", "steady", "unknown"} {
			ctx := phrase.Context{Band: band, Note: note}
			if phrase.Pick(c, ctx, alwaysCommon{}, nil) != "" {
				spoke = true
				continue
			}
			// Not a failure on its own, but worth naming: this is a band and
			// trend the bird passes through in silence.
			t.Logf("silent: %s+%s has no line to draw", band, note)
		}
		if !spoke {
			t.Errorf("%s can never say anything", band)
		}
	}
}

func TestDeadIsPinnedAtExactlyOneLine(t *testing.T) {
	// VOICE.md rule 9. The silence after it is the loudest thing this tool
	// says, and a second line would be the tool talking over its own point.
	if got := phrase.FromFS(Corpus).Lines("states/dead.txt"); len(got) != 1 {
		t.Errorf("states/dead.txt has %d lines, want exactly 1: %q", len(got), got)
	}
}

func TestPhrasesObeyTheVoiceRules(t *testing.T) {
	c := phrase.FromFS(Corpus)
	for _, f := range corpusFiles(t) {
		for _, line := range c.Lines(f) {
			if w := runewidth.StringWidth(line); w > MaxCells {
				t.Errorf("%s: %d cells, over the %d budget: %q", f, w, MaxCells, line)
			}
			if r := []rune(line)[0]; unicode.IsUpper(r) {
				t.Errorf("%s: starts with a capital: %q", f, line)
			}
			lower := strings.ToLower(line)
			for _, b := range banned {
				if strings.Contains(lower, b) {
					t.Errorf("%s: contains %q, which nags rather than observes: %q", f, b, line)
				}
			}
			if hasEmoji(line) {
				t.Errorf("%s: contains an emoji: %q", f, line)
			}
			// Rule 3: the bird computes context, turns and durations, and never
			// prints one — a figure makes this a widget. It applies where the
			// bird is talking about your session; a year in a piece of lore is
			// a fact about the world, not a measurement of you.
			if aboutYou(f) && strings.ContainsAny(line, "0123456789") {
				t.Errorf("%s: prints a number about the session: %q", f, line)
			}
		}
	}
}

func TestLoreNeverLooksAtYou(t *testing.T) {
	// VOICE.md §6: lore is the section where the bird is not looking at you.
	c := phrase.FromFS(Corpus)
	for _, f := range corpusFiles(t) {
		if !strings.HasPrefix(f, "lore/") {
			continue
		}
		for _, line := range c.Lines(f) {
			for _, word := range strings.FieldsFunc(strings.ToLower(line), func(r rune) bool {
				return !unicode.IsLetter(r)
			}) {
				if word == "you" || word == "your" {
					t.Errorf("%s: lore addresses the reader: %q", f, line)
				}
			}
		}
	}
}

func TestNoPhraseAppearsTwice(t *testing.T) {
	// A duplicate halves the value of the roll that found it, and the corpus is
	// small enough that a repeat is noticeable.
	c := phrase.FromFS(Corpus)
	seen := map[string]string{}
	for _, f := range corpusFiles(t) {
		for _, line := range c.Lines(f) {
			key := strings.TrimRight(strings.ToLower(line), ".")
			if first, ok := seen[key]; ok {
				t.Errorf("%q appears in both %s and %s", line, first, f)
				continue
			}
			seen[key] = f
		}
	}
}

func TestStateFilesNameARealBandAndNote(t *testing.T) {
	// A typo in a filename is invisible: the pool simply never matches and the
	// bird is quiet in that state forever.
	bands := map[string]bool{"fresh": true, "chirpy": true, "tired": true, "worn": true, "dead": true}
	notes := map[string]bool{"rising": true, "falling": true, "steady": true, "unknown": true}

	for _, f := range corpusFiles(t) {
		dir, name := path.Split(f)
		name = strings.TrimSuffix(name, ".txt")
		switch dir {
		case "states/":
			band, note, hasNote := strings.Cut(name, "+")
			if !bands[band] {
				t.Errorf("states/%s.txt: %q is not a band", name, band)
			}
			if hasNote && !notes[note] {
				t.Errorf("states/%s.txt: %q is not a note", name, note)
			}
		case "notes/":
			if !notes[name] {
				t.Errorf("notes/%s.txt: %q is not a note", name, name)
			}
		}
	}
}

// aboutYou reports whether a file is the bird describing your session rather
// than the world outside the cage.
func aboutYou(f string) bool {
	for _, dir := range []string{"states/", "notes/", "returns/", "time/"} {
		if strings.HasPrefix(f, dir) {
			return true
		}
	}
	return false
}

// hasEmoji is deliberately coarse: the pictographic blocks, not a full
// grapheme-cluster analysis. The rule is "no emoji", and anything in these
// ranges is one.
func hasEmoji(s string) bool {
	for _, r := range s {
		switch {
		case r >= 0x1F300 && r <= 0x1FAFF,
			r >= 0x2600 && r <= 0x27BF,
			r == 0xFE0F:
			return true
		}
	}
	return false
}
