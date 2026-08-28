// Package lint is VOICE.md §6: the mechanical half of a phrase review.
//
// It exists so a corpus PR can be a three-second yes instead of an argument
// about tone. Everything objective is checked here — width, case, nagging
// verbs, duplicates, filenames that nothing will ever read — and review is left
// with the only question that needs a person: is the line any good.
//
// Both `canary lint` and `go test ./...` run this exact code. A check that only
// runs in one of them is a check that eventually only runs in neither.
package lint

import (
	"path"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/mattn/go-runewidth"
	"github.com/thousandflowers/canary/internal/phrase"
	"github.com/thousandflowers/canary/internal/session"
)

// MaxCells is the width budget. The bird's own row costs 12 cells before the
// phrase starts, so this leaves a line readable on a 90-column terminal without
// render.Fit ever having to cut it.
const MaxCells = 78

// NearDuplicateEdits is how many single-character edits apart two lines may be
// before they are the same joke told twice. Three catches a retyped line with a
// different article or a fixed typo; it does not catch two short lines that
// merely rhyme.
const NearDuplicateEdits = 3

// MinNearDuplicateLen keeps the near-duplicate check off short lines, where
// three edits is most of the phrase.
const MinNearDuplicateLen = 20

// banned are the phrasings that turn an observation into nagging (VOICE.md
// rules 2 and 4). The bird notices; it does not instruct.
var banned = []string{"should", "must", "you need", "take a break", "remember to", "!"}

// knownSlots are the values the renderer can actually supply. A template asking
// for anything else resolves to nothing and is silently skipped forever.
var knownSlots = map[string]bool{"file": true, "repo": true, "time": true, "n": true}

var (
	bands = map[string]bool{"fresh": true, "chirpy": true, "tired": true, "worn": true, "dead": true}
	notes = map[string]bool{"rising": true, "falling": true, "steady": true, "unknown": true}
)

// Finding is one problem with one line, or with one file.
type Finding struct {
	File    string
	Line    string // the phrase itself, empty when the finding is about the file
	Problem string
}

func (f Finding) String() string {
	if f.Line == "" {
		return f.File + ": " + f.Problem
	}
	return f.File + ": " + f.Problem + ": " + strconv.Quote(f.Line)
}

// Check runs every rule over a corpus and returns what it found, in a stable
// order.
func Check(c phrase.Corpus) []Finding {
	files := c.All()
	var out []Finding
	out = append(out, checkFiles(c, files)...)
	out = append(out, checkPhrases(c, files)...)
	out = append(out, checkDuplicates(c, files)...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		return out[i].Problem < out[j].Problem
	})
	return out
}

// checkFiles is about names and shapes: a file nothing will ever read is worse
// than a bad line, because nobody finds out for months.
func checkFiles(c phrase.Corpus, files []string) []Finding {
	var out []Finding
	for _, f := range files {
		dir, name := path.Split(f)
		name = strings.TrimSuffix(name, ".txt")
		// The language directory is not part of the category.
		if i := strings.Index(dir, "/"); i >= 0 && dir[:i] == phrase.DefaultLang {
			dir = dir[i+1:]
		}

		switch dir {
		case "states/":
			band, note, hasNote := strings.Cut(name, "+")
			if !bands[band] {
				out = append(out, Finding{File: f, Problem: strconv.Quote(band) + " is not a band, so nothing will ever read this file"})
			}
			if hasNote && !notes[note] {
				out = append(out, Finding{File: f, Problem: strconv.Quote(note) + " is not a note"})
			}
			if band == "dead" && len(c.Lines(f)) != 1 {
				out = append(out, Finding{File: f, Problem: "dead is pinned at exactly one line (VOICE.md rule 9): the silence after it is the point"})
			}
		case "notes/":
			if !notes[name] {
				out = append(out, Finding{File: f, Problem: strconv.Quote(name) + " is not a note"})
			}
		case "triggers/":
			if !contains(session.TriggerNames, name) {
				out = append(out, Finding{File: f, Problem: "no detector for this trigger, so nothing will ever draw from it — triggers are code, see CONTRIBUTING"})
			}
		case "ephemeral/":
			if y, err := strconv.Atoi(name); err != nil || y < 2000 || y > 2999 {
				out = append(out, Finding{File: f, Problem: "ephemeral files are named for a year; this one will never be read"})
			}
		}
	}
	return out
}

func checkPhrases(c phrase.Corpus, files []string) []Finding {
	var out []Finding
	for _, f := range files {
		aboutYou := isAboutYou(f)
		isLore := strings.Contains(f, "lore/")
		for _, line := range c.Lines(f) {
			add := func(problem string) { out = append(out, Finding{File: f, Line: line, Problem: problem}) }

			if w := runewidth.StringWidth(line); w > MaxCells {
				add(strconv.Itoa(w) + " cells, over the " + strconv.Itoa(MaxCells) + " budget")
			}
			if r := []rune(line)[0]; unicode.IsUpper(r) {
				add("starts with a capital")
			}
			lower := strings.ToLower(line)
			for _, b := range banned {
				if strings.Contains(lower, b) {
					add("contains " + strconv.Quote(b) + ", which nags rather than observes")
				}
			}
			if hasEmoji(line) {
				add("contains an emoji")
			}
			if hasRTL(line) {
				// Arabic and Hebrew in a status row shared with another segment
				// produce bidi corruption nobody can control.
				add("right-to-left text: LTR scripts only, mine/ included")
			}
			// Rule 3: the bird computes durations and never prints one — a
			// figure makes it a widget. It applies where the bird is talking
			// about your session; a year in a piece of lore is a fact about the
			// world, not a measurement of you.
			if aboutYou && strings.ContainsAny(line, "0123456789") {
				add("prints a number about the session")
			}
			if isLore {
				for _, w := range words(lower) {
					if w == "you" || w == "your" {
						add("lore addresses the reader; that is the section where the bird is not looking at you")
						break
					}
				}
			}
			for _, slot := range phrase.SlotsIn(line) {
				if !knownSlots[slot] {
					add("unknown slot {" + slot + "}: nothing can fill it, so the line is skipped forever")
				}
			}
			if len(phrase.SlotsIn(line)) > 0 && !phrase.HasFallback(line) {
				add("template with no fallback: add ` | ` and a version that needs no slots")
			}
		}
	}
	return out
}

// checkDuplicates catches the same line twice, and the same line retyped. A
// duplicate halves the value of the draw that found it, and the corpus is small
// enough that a repeat is noticeable.
func checkDuplicates(c phrase.Corpus, files []string) []Finding {
	type entry struct{ file, norm, line string }
	var all []entry
	for _, f := range files {
		for _, line := range c.Lines(f) {
			all = append(all, entry{file: f, norm: normalize(line), line: line})
		}
	}

	var out []Finding
	seen := map[string]string{}
	for _, e := range all {
		if first, ok := seen[e.norm]; ok {
			out = append(out, Finding{File: e.file, Line: e.line, Problem: "already in " + first})
			continue
		}
		seen[e.norm] = e.file
	}

	for i := 0; i < len(all); i++ {
		for j := i + 1; j < len(all); j++ {
			a, b := all[i], all[j]
			if a.norm == b.norm {
				continue // already reported as an exact duplicate
			}
			if len(a.norm) < MinNearDuplicateLen || len(b.norm) < MinNearDuplicateLen {
				continue
			}
			if abs(len(a.norm)-len(b.norm)) > NearDuplicateEdits {
				continue // too far apart in length to be within the edit budget
			}
			if editDistance(a.norm, b.norm) <= NearDuplicateEdits {
				out = append(out, Finding{File: b.file, Line: b.line, Problem: "nearly the same as a line in " + a.file + ": " + strconv.Quote(a.line)})
			}
		}
	}
	return out
}

// isAboutYou reports whether a file is the bird describing your session rather
// than the world outside the cage.
func isAboutYou(f string) bool {
	for _, dir := range []string{"states/", "notes/", "returns/", "time/", "triggers/"} {
		if strings.Contains(f, dir) {
			return true
		}
	}
	return false
}

func normalize(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' {
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// editDistance is Levenshtein with a rolling row: the corpus is small, but this
// runs over every pair, and the square is the part worth keeping cheap.
func editDistance(a, b string) int {
	ar, br := []rune(a), []rune(b)
	prev := make([]int, len(br)+1)
	cur := make([]int, len(br)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ar); i++ {
		cur[0] = i
		for j := 1; j <= len(br); j++ {
			cost := 1
			if ar[i-1] == br[j-1] {
				cost = 0
			}
			cur[j] = min3(cur[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(br)]
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

// hasRTL reports whether a line contains Hebrew, Arabic, Syriac or Thaana.
func hasRTL(s string) bool {
	for _, r := range s {
		switch {
		case r >= 0x0590 && r <= 0x08FF,
			r >= 0xFB1D && r <= 0xFDFF,
			r >= 0xFE70 && r <= 0xFEFF:
			return true
		}
	}
	return false
}

func words(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) })
}

func contains(hay []string, needle string) bool {
	for _, s := range hay {
		if s == needle {
			return true
		}
	}
	return false
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func min3(a, b, c int) int {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}
