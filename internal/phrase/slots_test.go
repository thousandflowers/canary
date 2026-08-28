package phrase

import "testing"

func TestALineWithNoSlotsIsLeftAlone(t *testing.T) {
	// Templates are for the handful of lines that need context, not a format
	// every contributor has to learn.
	const line = "the air is breathable. i'm bored."
	if got := Resolve(line, Slots{}); got != line {
		t.Errorf("got %q, want the line untouched", got)
	}
}

func TestSlotsAreFilled(t *testing.T) {
	got := Resolve("{repo} has been open since {time}.", Slots{"repo": "canary", "time": "this morning"})
	if got != "canary has been open since this morning." {
		t.Errorf("got %q", got)
	}
}

func TestAnUnfillableTemplateFallsToItsFallback(t *testing.T) {
	// Never print a broken row.
	got := Resolve("{repo} has been open since {time}. | this has been open a while.", Slots{})
	if got != "this has been open a while." {
		t.Errorf("got %q, want the fallback", got)
	}
}

func TestATemplateWithNoWayOutIsSkipped(t *testing.T) {
	if got := Resolve("{file} has been rewritten {n} times.", Slots{"file": "state.go"}); got != "" {
		t.Errorf("got %q, want nothing at all", got)
	}
}

func TestAnEmptyValueIsAMissingValue(t *testing.T) {
	if got := Resolve("{repo} is quiet. | something is quiet.", Slots{"repo": "   "}); got != "something is quiet." {
		t.Errorf("got %q, want the fallback", got)
	}
}

func TestTimeIsAWordNotAFigure(t *testing.T) {
	// VOICE.md rule 3: the bird computes durations and never prints one,
	// because a figure makes it a widget.
	cases := map[int]string{2: "the middle of the night", 9: "this morning", 13: "before lunch", 16: "this afternoon", 20: "this evening", 23: "tonight"}
	for hour, want := range cases {
		if got := (Slots{}).Time(hour)["time"]; got != want {
			t.Errorf("hour %d: got %q, want %q", hour, got, want)
		}
	}
}

func TestCountIsOnlyOfferedWhenThereIsOne(t *testing.T) {
	if _, ok := (Slots{}).Count(0)["n"]; ok {
		t.Error("a count of zero was offered as a value")
	}
	if got := (Slots{}).Count(6)["n"]; got != "6" {
		t.Errorf("got %q, want 6", got)
	}
}

func TestSlotsInAndHasFallback(t *testing.T) {
	if got := SlotsIn("{repo} at {time} | plain"); len(got) != 2 || got[0] != "repo" || got[1] != "time" {
		t.Errorf("SlotsIn: %q", got)
	}
	if !HasFallback("{repo} is open | this is open") {
		t.Error("a slot-free last alternative is a fallback")
	}
	if HasFallback("{repo} is open | {repo} is still open") {
		t.Error("an alternative that needs the same slot is not a way out")
	}
	if !HasFallback("a line with no slots at all") {
		t.Error("a line with no slots never needs one")
	}
}

func TestAnUnclosedBraceIsATypoNotASlot(t *testing.T) {
	// The linter is the right place to complain; the renderer prints what is
	// written rather than swallowing the line.
	const line = "a { that never closes."
	if got := Resolve(line, Slots{}); got != line {
		t.Errorf("got %q, want the line as written", got)
	}
}
