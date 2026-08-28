// Package config resolves the env-var knobs and the paths under ~/.canary once,
// so no other package has to reach for os.Getenv and guess a default.
//
// Every knob here existed first as a shell variable read inline at its point of
// use, which is how canary.sh and canary-statusline.sh drifted apart: one
// treated CANARY_DISABLED as "non-empty", the other as "equals 1", and
// CANARY_SHOW_SCORE had the same split. One parser, one meaning.
package config

import (
	"os"
	"path/filepath"
	"strconv"

	"github.com/thousandflowers/canary/internal/fatigue"
)

// Defaults that have no home in the fatigue package because they are about
// drawing or timing rather than the score itself.
const (
	DefaultIdleThreshold = 300 // seconds; a longer gap is a break, not work
	DefaultColumns       = 80  // when the terminal will not say how wide it is
)

// Config is the whole tunable surface, resolved.
type Config struct {
	// Paths
	Home        string
	StateFile   string
	HistoryFile string
	PhraseState string
	RecentFile  string
	BagFile     string
	SessionFile string
	GitCache    string
	PhraseDir   string // empty means "use the embedded corpus"

	// Behaviour
	Disabled      bool
	Quiet         bool
	ASCII         bool
	ShowScore     bool
	DeadAbsolute  bool
	MinScore      int
	IdleThreshold int
	ReserveCols   int
	Columns       int

	// Score weights
	NightMult int
	ErrWeight int
	RepWeight int
	DebtMax   int
}

// FromEnv reads the environment. It never fails: a knob set to nonsense falls
// back to its default rather than taking the bird down, because this runs on
// every prompt and every statusline refresh, and a crash there is far worse
// than an ignored typo in an rc file.
func FromEnv() Config {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.Getenv("HOME")
	}
	dir := filepath.Join(home, ".canary")

	c := Config{
		Home:        home,
		StateFile:   envPath("CANARY_STATE_FILE", filepath.Join(dir, "canary-state")),
		HistoryFile: envPath("CANARY_HISTORY_FILE", filepath.Join(dir, "history")),
		PhraseState: envPath("CANARY_PHRASE_STATE", filepath.Join(dir, "phrase-state")),
		RecentFile:  envPath("CANARY_RECENT_FILE", filepath.Join(dir, "recent")),
		// The shuffle state, the sessions seen today, and the last answer git
		// gave about the working tree. All three exist so the hot path can stay
		// off the disk and off `git status` on most refreshes.
		BagFile:     envPath("CANARY_BAG_FILE", filepath.Join(dir, "bag.json")),
		SessionFile: envPath("CANARY_SESSION_FILE", filepath.Join(dir, "sessions")),
		GitCache:    envPath("CANARY_GIT_CACHE", filepath.Join(dir, "git-cache")),

		Disabled:     truthy(os.Getenv("CANARY_DISABLED")),
		Quiet:        truthy(os.Getenv("CANARY_QUIET")),
		ASCII:        truthy(os.Getenv("CANARY_ASCII")),
		ShowScore:    truthy(os.Getenv("CANARY_SHOW_SCORE")),
		DeadAbsolute: truthy(os.Getenv("CANARY_DEAD_ABSOLUTE")),

		MinScore:      envInt("CANARY_MIN_SCORE", 0),
		IdleThreshold: envInt("CANARY_IDLE_THRESHOLD", DefaultIdleThreshold),
		ReserveCols:   envInt("CANARY_RESERVE_COLS", 0),
		Columns:       envInt("COLUMNS", DefaultColumns),

		NightMult: envInt("CANARY_NIGHT_MULT", fatigue.DefaultNightMult),
		ErrWeight: envInt("CANARY_ERR_WEIGHT", fatigue.DefaultErrWeight),
		RepWeight: envInt("CANARY_REP_WEIGHT", fatigue.DefaultRepWeight),
		DebtMax:   envInt("CANARY_DEBT_MAX", fatigue.DefaultDebtMax),
	}

	// The corpus lives in the binary. A directory is an override for people
	// editing phrases — a contributor iterating on a line should see it without
	// rebuilding — so it only counts when it actually holds a corpus.
	if d := os.Getenv("CANARY_PHRASE_DIR"); d != "" {
		c.PhraseDir = d
	} else if d := filepath.Join(dir, "phrases"); isDir(filepath.Join(d, "en")) {
		c.PhraseDir = d
	}

	if c.Columns <= 0 {
		c.Columns = DefaultColumns
	}
	return c
}

// Dir is ~/.canary, the directory every generated file lives under.
func (c Config) Dir() string { return filepath.Dir(c.StateFile) }

// truthy accepts any non-empty value except "0", which reconciles the two
// readings the shell scripts had. `CANARY_DISABLED=` (empty) is off, matching
// `unset`; anything a person would type meaning yes is on.
func truthy(v string) bool { return v != "" && v != "0" }

func envInt(key string, def int) int {
	n, err := strconv.Atoi(os.Getenv(key))
	if err != nil {
		return def
	}
	return n
}

func envPath(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}
