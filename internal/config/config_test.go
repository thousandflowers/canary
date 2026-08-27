package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/thousandflowers/canary/internal/fatigue"
)

func TestDefaults(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	c := FromEnv()

	if c.NightMult != fatigue.DefaultNightMult || c.ErrWeight != fatigue.DefaultErrWeight ||
		c.RepWeight != fatigue.DefaultRepWeight || c.DebtMax != fatigue.DefaultDebtMax {
		t.Errorf("score weights drifted from the fatigue package: %+v", c)
	}
	if c.IdleThreshold != DefaultIdleThreshold {
		t.Errorf("idle threshold = %d, want %d", c.IdleThreshold, DefaultIdleThreshold)
	}
	if c.MinScore != 0 {
		t.Errorf("the bird draws at every score by default, got a floor of %d", c.MinScore)
	}
	if c.Disabled || c.Quiet || c.ASCII {
		t.Errorf("a knob defaulted to on: %+v", c)
	}
}

func TestTruthyIsOneReadingNotTwo(t *testing.T) {
	// canary.sh tested CANARY_DISABLED for non-empty and the statusline tested
	// it for "1", so `CANARY_DISABLED=yes` silenced one bird and not the other.
	for _, v := range []string{"1", "yes", "true", "0 "} {
		t.Setenv("CANARY_DISABLED", v)
		if !FromEnv().Disabled {
			t.Errorf("CANARY_DISABLED=%q did not silence the bird", v)
		}
	}
	for _, v := range []string{"", "0"} {
		t.Setenv("CANARY_DISABLED", v)
		if FromEnv().Disabled {
			t.Errorf("CANARY_DISABLED=%q silenced the bird", v)
		}
	}
}

func TestNonsenseFallsBackInsteadOfCrashing(t *testing.T) {
	// This runs on every prompt. A typo in an rc file must not take the shell
	// down with it.
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CANARY_MIN_SCORE", "worn")
	t.Setenv("CANARY_NIGHT_MULT", "")
	t.Setenv("COLUMNS", "-4")

	c := FromEnv()
	if c.MinScore != 0 {
		t.Errorf("MinScore = %d, want the default", c.MinScore)
	}
	if c.NightMult != fatigue.DefaultNightMult {
		t.Errorf("NightMult = %d, want the default", c.NightMult)
	}
	if c.Columns != DefaultColumns {
		t.Errorf("Columns = %d, want %d when the terminal says something absurd", c.Columns, DefaultColumns)
	}
}

func TestPathsFollowHomeAndTheirOverrides(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	c := FromEnv()
	if c.StateFile != filepath.Join(home, ".canary", "canary-state") {
		t.Errorf("state file = %s", c.StateFile)
	}

	t.Setenv("CANARY_STATE_FILE", "/tmp/elsewhere")
	if got := FromEnv().StateFile; got != "/tmp/elsewhere" {
		t.Errorf("override ignored: %s", got)
	}
}

func TestTheCorpusOnlyOverridesWhenThereIsOne(t *testing.T) {
	// The binary carries the corpus. A directory wins only when it actually
	// holds one, so an empty ~/.canary/phrases cannot mute the bird — which is
	// exactly how the shell version shipped mute.
	home := t.TempDir()
	t.Setenv("HOME", home)
	os.MkdirAll(filepath.Join(home, ".canary", "phrases"), 0o755)

	if got := FromEnv().PhraseDir; got != "" {
		t.Errorf("an empty phrases dir overrode the embedded corpus: %s", got)
	}

	os.MkdirAll(filepath.Join(home, ".canary", "phrases", "en"), 0o755)
	if got := FromEnv().PhraseDir; got == "" {
		t.Error("a real corpus on disk did not override the embedded one")
	}

	t.Setenv("CANARY_PHRASE_DIR", "/somewhere/else")
	if got := FromEnv().PhraseDir; got != "/somewhere/else" {
		t.Errorf("an explicit corpus root was ignored: %s", got)
	}
}
