package phrase

import (
	"strconv"
	"strings"
)

// Templates carry their own fallback:
//
//	{repo} has been open since {time}. | this has been open a while.
//
// The alternatives are tried left to right and the first one whose slots all
// resolve is used. A line whose every alternative needs something the bird does
// not know is skipped and another is drawn — never printed with a hole in it.
//
// The separator is " | " with spaces, so a phrase can still contain a bare
// pipe if it ever needs one.
const altSep = " | "

// Slots are the values available to a template on this refresh. A missing key
// and an empty value mean the same thing: this alternative cannot be used.
type Slots map[string]string

// Time turns the hour into a word rather than a figure. VOICE rule 3: the bird
// computes durations and never prints one, because a number makes it a widget.
func (s Slots) Time(hour int) Slots {
	switch {
	case hour < 5:
		s["time"] = "the middle of the night"
	case hour < 12:
		s["time"] = "this morning"
	case hour < 14:
		s["time"] = "before lunch"
	case hour < 18:
		s["time"] = "this afternoon"
	case hour < 22:
		s["time"] = "this evening"
	default:
		s["time"] = "tonight"
	}
	return s
}

// Count fills {n}.
func (s Slots) Count(n int) Slots {
	if n > 0 {
		s["n"] = strconv.Itoa(n)
	}
	return s
}

// Resolve fills a template, or returns "" if no alternative can be filled.
//
// A line with no slots at all is returned as it is, which is the overwhelming
// majority of the corpus: templates are a feature for the handful of lines that
// need context, not a format everyone has to learn.
func Resolve(line string, slots Slots) string {
	for _, alt := range strings.Split(line, altSep) {
		alt = strings.TrimSpace(alt)
		if alt == "" {
			continue
		}
		if out, ok := fill(alt, slots); ok {
			return out
		}
	}
	return ""
}

// fill substitutes every {slot}; ok is false as soon as one is unknown.
func fill(line string, slots Slots) (string, bool) {
	var b strings.Builder
	rest := line
	for {
		open := strings.IndexByte(rest, '{')
		if open < 0 {
			b.WriteString(rest)
			return b.String(), true
		}
		close := strings.IndexByte(rest[open:], '}')
		if close < 0 {
			// An unclosed brace is a typo, not a slot. Print it as written and
			// let the linter be the one to complain.
			b.WriteString(rest)
			return b.String(), true
		}
		close += open

		name := rest[open+1 : close]
		value := slots[name]
		if strings.TrimSpace(value) == "" {
			return "", false
		}
		b.WriteString(rest[:open])
		b.WriteString(value)
		rest = rest[close+1:]
	}
}

// SlotsIn lists the slot names a line uses, across every alternative. The
// linter needs this to check that a template with slots has a way out.
func SlotsIn(line string) []string {
	var out []string
	rest := line
	for {
		open := strings.IndexByte(rest, '{')
		if open < 0 {
			return out
		}
		close := strings.IndexByte(rest[open:], '}')
		if close < 0 {
			return out
		}
		close += open
		if name := rest[open+1 : close]; name != "" {
			out = append(out, name)
		}
		rest = rest[close+1:]
	}
}

// HasFallback reports whether a template ends in an alternative that needs no
// slots at all — the one that can always be printed.
func HasFallback(line string) bool {
	alts := strings.Split(line, altSep)
	if len(alts) < 2 {
		return len(SlotsIn(line)) == 0
	}
	return len(SlotsIn(alts[len(alts)-1])) == 0
}
