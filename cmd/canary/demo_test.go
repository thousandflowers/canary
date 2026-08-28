package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thousandflowers/canary/internal/config"
	"github.com/thousandflowers/canary/internal/fatigue"
	"github.com/thousandflowers/canary/internal/phrase"
	"github.com/thousandflowers/canary/internal/render"
)

// noSleep runs the demo at full speed. Everything below asserts frames, not
// timing: the pace is one constant and the tape is what decides how it reads.
func noSleep(t *testing.T) {
	t.Helper()
	old := demoTick
	demoTick = 0
	t.Cleanup(func() { demoTick = old })
}

func TestDemoUsage(t *testing.T) {
	isolate(t)
	for _, args := range [][]string{
		{"--fast"},                    // a flag that does not exist
		{"--band", "worn"},            // the right shape, the wrong flag
		{"--state", "worn", "--fast"}, // more than it takes
		{"--state", "exhausted"},      // a band that does not exist
	} {
		if _, code := capture(t, func() int { return runDemo(config.FromEnv(), args) }); code != 2 {
			t.Errorf("demo %v should be a usage error", args)
		}
	}
}

func TestOneStateOnItsOwnReportsNoSession(t *testing.T) {
	// A single state is an example, not a session. Printing minutes and turns
	// beside it would be inventing a session that is not happening.
	isolate(t)
	noSleep(t)

	for _, band := range []string{"fresh", "chirpy", "tired", "worn", "dead"} {
		out, code := capture(t, func() int { return runDemo(config.FromEnv(), []string{"--state", band}) })
		if code != 0 {
			t.Fatalf("demo --state %s: exit %d", band, code)
		}
		// The middle dot also lives in the animation frames, so look for the
		// shape only the status row has: minutes and turns.
		if strings.Contains(out, "m · ") {
			t.Errorf("--state %s drew a status row:\n%s", band, out)
		}
		// And only that state: no other band's face appears.
		for other, face := range map[string]string{
			"fresh": "▐ O ▌>", "chirpy": "▐ ^ ▌>", "tired": "▐ - ▌>", "worn": "▐ ~ ▌>", "dead": "▐ x ▌v",
		} {
			if other != band && strings.Contains(out, face) {
				t.Errorf("--state %s also drew %s", band, other)
			}
		}
	}
}

func TestTheStatusRowSaysSomethingTrue(t *testing.T) {
	// Every pair in demoSession has to actually produce the band it is listed
	// under. A demo whose numbers do not add up teaches the wrong thing about
	// the only figures canary ever prints.
	for band, session := range demoSession {
		raw := fatigue.ClaudeCodeRaw(session.Minutes, session.Turns, 0, 0, 3, 2)
		if got := fatigue.BandFor(fatigue.Cap(raw)); got != band {
			t.Errorf("%dm and %dt score %d, which is %s, not %s",
				session.Minutes, session.Turns, fatigue.Cap(raw), got, band)
		}
	}
}

func TestDemoDrawsTheWholeBirdAndNothingElse(t *testing.T) {
	isolate(t)
	noSleep(t)

	out, code := capture(t, func() int { return runDemo(config.FromEnv(), nil) })
	if code != 0 {
		t.Fatalf("exit %d", code)
	}

	// It clears the screen and hides the cursor: the demo is the only thing on
	// camera, and a blinking block in the middle of the bird is not the bird.
	for _, want := range []string{"\033[2J", "\033[?25l", "\033[?25h"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing terminal control %q", want)
		}
	}
	// Every band's face, in the order the bird actually walks.
	order := []string{"▐ O ▌>", "▐ ^ ▌>", "▐ - ▌>", "▐ ~ ▌>", "▐ x ▌v"}
	at := 0
	for _, face := range order {
		i := strings.Index(out[at:], face)
		if i < 0 {
			t.Fatalf("%s never appeared, or appeared out of order", face)
		}
		at += i
	}
	// And it speaks: the slot beside the beak carries real corpus lines.
	if !strings.Contains(out, "⌐ ") {
		t.Error("the bird never said anything")
	}
	if !strings.Contains(out, "the canary is quiet.") {
		t.Error("the dead bird did not say its one line")
	}
}

func TestTheDemoIsTheRealCorpusAndTheRealArt(t *testing.T) {
	// Not a script of hardcoded strings: a demo that drifts from the thing it
	// demonstrates is worse than no demo.
	home := isolate(t)
	dir := filepath.Join(home, "corpus")
	for _, band := range []string{"fresh", "chirpy", "tired", "worn", "dead"} {
		os.MkdirAll(filepath.Join(dir, "en", "states"), 0o755)
		os.WriteFile(filepath.Join(dir, "en", "states", band+".txt"),
			[]byte("the "+band+" line.\n"), 0o644)
	}
	t.Setenv("CANARY_PHRASE_DIR", dir)
	noSleep(t)

	out, _ := capture(t, func() int { return runDemo(config.FromEnv(), nil) })
	for _, band := range []string{"fresh", "chirpy", "tired", "worn", "dead"} {
		if !strings.Contains(out, "the "+band+" line.") {
			t.Errorf("%s drew a line that is not in the corpus it was given", band)
		}
	}
}

func TestEveryBandMovesExceptTheDeadOne(t *testing.T) {
	// "manca l'animazione" was the complaint about the recording that showed
	// the prompt bird, which has no note at all. Every living band animates
	// here, each with its own pattern, and the dead one does not.
	cfg := config.Config{Columns: 100}
	frames := demoSequence(corpus(cfg), globalRand{}, cfg, nil)

	for _, band := range []fatigue.Band{fatigue.Fresh, fatigue.Chirpy, fatigue.Tired, fatigue.Worn} {
		// A band is identified by its eye; a frame it moved through is one that
		// ends in one of that band's animation frames.
		eye := "▐ " + render.ArtFor(band).Eye + " ▌"
		seen := map[string]bool{}
		for _, drawn := range frames {
			if !strings.Contains(drawn, eye) {
				continue
			}
			for _, f := range render.Frames(band, false) {
				if strings.HasSuffix(drawn, "  "+f) {
					seen[f] = true
				}
			}
		}
		if len(seen) < 2 {
			t.Errorf("%s never moved: saw %d distinct animation frames", band, len(seen))
		}
	}

	// The dead bird's last word is its line, and then nothing beside the beak.
	last := frames[len(frames)-1]
	if !strings.HasSuffix(last, "▐ x ▌v") {
		t.Errorf("the demo does not end in silence:\n%q", last)
	}
}

func TestTheDemoWritesNothing(t *testing.T) {
	// Watching the bird must not age it.
	home := isolate(t)
	noSleep(t)
	capture(t, func() int { return runDemo(config.FromEnv(), nil) })

	if _, err := os.Stat(filepath.Join(home, ".canary")); err == nil {
		t.Error("the demo left state behind")
	}
}

func TestDemoLineFallsSilentWithNothingToSay(t *testing.T) {
	home := isolate(t)
	empty := filepath.Join(home, "empty")
	os.MkdirAll(filepath.Join(empty, "en"), 0o755)
	t.Setenv("CANARY_PHRASE_DIR", empty)

	if got := demoLine(phrase.FromDir(empty), fatigue.Worn, globalRand{}); got != "" {
		t.Errorf("got %q from a corpus with no lines in it", got)
	}
}
