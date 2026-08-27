// Package render draws the bird.
//
// Two surfaces, one set of art: the shell prompt, where the bird perches above
// the line you type, and Claude Code's status row, where it shares a line with
// whatever else lives there. They differ in framing only — the bands, the eyes
// and the beak are the same bird, and the suites have always asserted that
// agreement rather than either drawing in isolation.
package render

import (
	"fmt"
	"strings"

	"github.com/mattn/go-runewidth"
	"github.com/thousandflowers/canary/internal/fatigue"
)

// The slot to the right of the beak is one slot, shared: the note glyph when
// the bird is silent, the phrase when it speaks, nothing at all when it is dead
// and done talking.
const (
	glyphTail  = "⌐"
	glyphNote  = "♪"
	asciiTail  = "-"
	asciiNote  = "."
	slotIndent = "  "
)

// fitOverhead is the cells the bird's own row costs before any phrase: 2 for
// Claude Code's continuation indent and 10 for "▐ O ▌>  ⌐ ".
const fitOverhead = 12

// minPhrase is the shortest stub worth drawing. A phrase cut to five characters
// is noise where silence was an option.
const minPhrase = 12

// Art is one band's two rows, without the slot.
type Art struct {
	Top  string // the head
	Eye  string
	Beak string
}

// ArtFor maps a band to its drawing.
func ArtFor(b fatigue.Band) Art {
	switch b {
	case fatigue.Chirpy:
		return Art{Top: "▗███▖", Eye: "^", Beak: ">"}
	case fatigue.Tired:
		return Art{Top: "▗███▖", Eye: "-", Beak: ">"}
	case fatigue.Worn:
		return Art{Top: "▗▓▓▓▖", Eye: "~", Beak: ">"}
	case fatigue.Dead:
		return Art{Top: "▗░░░▖", Eye: "x", Beak: "v"}
	default:
		return Art{Top: "▗███▖", Eye: "O", Beak: ">"}
	}
}

// Prompt draws the two-row bird that sits above the shell prompt.
//
// The dead bird carries a third line naming the way out. It is the only place
// canary tells you what to do rather than showing you where you are, and it
// earns that by being the band where fatigue-risk protocols call for a break.
func Prompt(b fatigue.Band, score int, showScore bool) string {
	a := ArtFor(b)
	// The prompt bird's left column is one space wider than the statusline's,
	// and the dead bird's cage has already crumbled on that side.
	left := "▐"
	if b == fatigue.Dead {
		left = "░"
	}
	top := " " + a.Top
	if b == fatigue.Chirpy {
		top += " " + glyphNote // the only band that sings unprompted
	}
	body := fmt.Sprintf("%s %s ▌%s", left, a.Eye, a.Beak)

	var sb strings.Builder
	sb.WriteString(top + "\n")
	if showScore {
		fmt.Fprintf(&sb, "%s  [%s %d]\n", body, b, score)
	} else {
		sb.WriteString(body + "\n")
	}
	if b == fatigue.Dead {
		sb.WriteString("  tweet… you look fried. reset with  canary reset\n")
	}
	return sb.String()
}

// Status is everything the status row draws.
type Status struct {
	Band      fatigue.Band
	Minutes   int
	Turns     int
	StatName  string // t for turns (Claude Code), p for prompts (shell)
	Errors    int
	Debt      int
	Score     int
	Nights    int
	Phrase    string
	ShowScore bool
	ASCII     bool
	Columns   int
	Reserve   int
}

// Statusline draws the stat line, the bird and whatever the bird is saying.
//
// No trailing newline, on purpose: Claude Code allows one status line command,
// so canary is appended to whatever else is there, and a newline at the end
// would push the next segment onto a row of its own.
func Statusline(s Status) string {
	a := ArtFor(s.Band)
	tail, note := glyphTail, glyphNote
	if s.ASCII {
		tail, note = asciiTail, asciiNote
	}

	slot := ""
	if s.Phrase != "" {
		if fit := Fit(s.Phrase, s.Columns, s.Reserve); fit != "" {
			slot = slotIndent + tail + " " + fit
		}
	}
	// A dead bird that has said its line says nothing else — not even the note.
	if slot == "" && s.Band != fatigue.Dead {
		slot = slotIndent + note
	}

	var sb strings.Builder
	if s.ShowScore {
		fmt.Fprintf(&sb, " %s · %dm · %d%s · %de · d%d · %d\n",
			s.Band, s.Minutes, s.Turns, s.StatName, s.Errors, s.Debt, s.Score)
	} else {
		fmt.Fprintf(&sb, " %s · %dm · %d%s\n", s.Band, s.Minutes, s.Turns, s.StatName)
	}
	fmt.Fprintf(&sb, "%s\n▐ %s ▌%s%s", a.Top, a.Eye, a.Beak, slot)

	// Escalation, decoupled from the face: the demotion can calm a dead bird to
	// worn, but a streak is the case you cannot feel, so the count keeps saying
	// so in words the art no longer shows.
	if s.Nights >= 2 {
		fmt.Fprintf(&sb, "\n✕ %d nights past your limit", s.Nights)
	}
	return sb.String()
}

// Fit truncates a phrase to the cells left on the bird's row.
//
// Cells, not characters: the shell measured with ${#text}, which counts a
// double-width CJK glyph and an emoji as one column each and wrapped the row
// whenever a phrase used them. Truncation is at a word boundary with no
// ellipsis — an ellipsis promises a rest of the sentence that is not coming —
// and a stub shorter than minPhrase is dropped, because silence reads better
// than a fragment.
//
// COLUMNS is the only honest width source here: `tput cols` cannot see the
// terminal from inside a status line command.
func Fit(text string, cols, reserve int) string {
	if cols <= 0 {
		cols = 80
	}
	avail := cols - fitOverhead - reserve
	if avail < minPhrase {
		return ""
	}
	if runewidth.StringWidth(text) <= avail {
		return text
	}

	// Walk to the last word boundary that still fits.
	width, cut := 0, -1
	for i, r := range text {
		if r == ' ' && width <= avail {
			cut = i
		}
		width += runewidth.RuneWidth(r)
		if width > avail {
			break
		}
	}
	if cut < 0 {
		return "" // one long unbreakable word: nothing honest to show
	}
	out := strings.TrimRight(text[:cut], " ")
	if runewidth.StringWidth(out) < minPhrase {
		return ""
	}
	return out
}
