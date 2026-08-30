package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/thousandflowers/canary/internal/chrono"
	"github.com/thousandflowers/canary/internal/config"
)

// chronoOffset resolves how far the time-of-day curve should be rotated for
// this person. An explicit CANARY_CHRONO_OFFSET wins — including a deliberate
// 0, which pins the textbook curve and is how you turn the learning off.
func chronoOffset(cfg config.Config) int {
	if cfg.ChronoOffsetSet {
		return cfg.ChronoOffset
	}
	off, _ := chrono.Load(cfg.ChronoFile).Offset()
	return off
}

// recordChrono notes that you were awake this hour. It writes at most once an
// hour: chrono.Record says whether anything actually changed, and on all but
// the first prompt of each hour it has not.
//
// Errors are dropped on purpose, as everywhere else on this path. Losing an
// hour out of a decaying histogram is not worth a line of noise between you and
// the command you are trying to run.
func recordChrono(cfg config.Config, now time.Time) {
	if next, changed := chrono.Record(chrono.Load(cfg.ChronoFile), int(now.Unix()), now.Hour()); changed {
		_ = chrono.Save(cfg.ChronoFile, next)
	}
}

// runChrono is `canary chrono`: what the bird has worked out about your body
// clock, and where that came from. Without it the rotation is invisible — the
// score just quietly differs from the documented curve, which is the kind of
// thing that reads as a bug.
func runChrono(cfg config.Config, args []string) int {
	if len(args) > 0 && args[0] == "--bootstrap" {
		return runChronoBootstrap(cfg)
	}
	if len(args) > 0 {
		fmt.Fprintf(os.Stderr, "canary chrono: unknown argument %q\n", args[0])
		return 2
	}

	l := chrono.Load(cfg.ChronoFile)
	fmt.Println(chronoChart(l))

	center, r, ok := l.Center()
	if !ok {
		fmt.Printf("\nNot enough shape yet (%d/%d evidence, R=%.2f).\n", l.Total(), chrono.MinTotal, r)
		fmt.Println("Using the textbook curve until then. `canary chrono --bootstrap` seeds it from macOS's own screen-time history.")
		return 0
	}

	off, _ := l.Offset()
	fmt.Printf("\nYour day centres on %s (R=%.2f).\n", clockHour(center), r)
	if cfg.ChronoOffsetSet {
		fmt.Printf("Learned offset %+dh, overridden to %+dh by CANARY_CHRONO_OFFSET.\n", off, cfg.ChronoOffset)
		off = cfg.ChronoOffset
	} else {
		fmt.Printf("Offset %+dh.\n", off)
	}
	fmt.Printf("The deepest trough sits at %02d:00-%02d:00 for you, not 02:00-04:00.\n",
		chrono.Shift(2, -off), chrono.Shift(4, -off))
	return 0
}

// chronoChart draws the histogram. A column per hour, scaled to the busiest,
// so the sleep window is visible as the gap it is.
func chronoChart(l chrono.Log) string {
	ramp := []rune(" ▁▂▃▄▅▆▇█")
	peak := 0
	for _, n := range l.Slots {
		if n > peak {
			peak = n
		}
	}

	var bars strings.Builder
	for _, n := range l.Slots {
		i := 0
		if peak > 0 {
			i = n * (len(ramp) - 1) / peak
		}
		bars.WriteRune(ramp[i])
	}

	return "  " + bars.String() + "\n  0     6     12    18\n  hours you are awake in, decayed daily"
}

// clockHour renders a fractional hour as a time.
func clockHour(h float64) string {
	m := int(h*60+0.5) % (24 * 60)
	return fmt.Sprintf("%02d:%02d", m/60, m%60)
}

// --- bootstrap -------------------------------------------------------------

// knowledgeDB is macOS's own record of when the screen was in use. canary needs
// weeks of evidence before it will rotate anything, and this is weeks of
// exactly that evidence, already collected.
const knowledgeDB = "Library/Application Support/Knowledge/knowledgeC.db"

// hourCountsQuery counts the distinct day-hours in which any app was in use.
// Distinct day-hours, not events: the question is which hours you are awake in,
// and an afternoon of heavy app-switching is one afternoon, not four hundred.
//
// ZSTARTDATE is Core Data time — seconds since 2001-01-01 — hence the constant.
// The stream is /app/usage; /app/inFocus is the iOS spelling and matches
// nothing here.
const hourCountsQuery = `SELECT h, COUNT(*) FROM (
  SELECT DISTINCT
    strftime('%Y%m%d%H', datetime(ZSTARTDATE+978307200,'unixepoch','localtime')) AS dh,
    CAST(strftime('%H', datetime(ZSTARTDATE+978307200,'unixepoch','localtime')) AS INTEGER) AS h
  FROM ZOBJECT WHERE ZSTREAMNAME='/app/usage' AND ZSTARTDATE IS NOT NULL
) GROUP BY h ORDER BY h;`

const dayCountQuery = `SELECT COUNT(DISTINCT date(datetime(ZSTARTDATE+978307200,'unixepoch','localtime')))
  FROM ZOBJECT WHERE ZSTREAMNAME='/app/usage' AND ZSTARTDATE IS NOT NULL;`

func runChronoBootstrap(cfg config.Config) int {
	if runtime.GOOS != "darwin" {
		fmt.Fprintln(os.Stderr, "canary chrono --bootstrap: macOS only; elsewhere the log fills itself in over a few weeks")
		return 1
	}
	db := filepath.Join(cfg.Home, knowledgeDB)
	if _, err := os.Stat(db); err != nil {
		fmt.Fprintf(os.Stderr, "canary chrono --bootstrap: no screen-time history at %s\n", db)
		return 1
	}

	counts, days, err := knowledgeHours(db)
	if err != nil {
		fmt.Fprintf(os.Stderr, "canary chrono --bootstrap: %v\n", err)
		fmt.Fprintln(os.Stderr, "If that is a permissions error, your terminal needs Full Disk Access in System Settings > Privacy & Security.")
		return 1
	}

	seeded := chrono.Seed(counts, days)
	if seeded.Total() < chrono.MinTotal {
		fmt.Fprintf(os.Stderr, "canary chrono --bootstrap: only %d days of history, not enough to seed from\n", days)
		return 1
	}

	// Seeding replaces rather than merges. The two sources count different
	// things — one is every app on the machine, the other is canary's own
	// prompts — and adding them would weight whichever happened to be larger.
	now := time.Now()
	seeded.Day = int(now.Unix()) / 86400
	if err := chrono.Save(cfg.ChronoFile, seeded); err != nil {
		return fail(err)
	}

	fmt.Printf("Seeded from %d days of macOS screen-time history.\n\n", days)
	return runChrono(cfg, nil)
}

// knowledgeHours asks sqlite3 for the histogram. Shelling out rather than
// linking a driver: sqlite3 ships with macOS, the query runs once by hand, and
// a cgo dependency for that is a bad trade on a binary that otherwise builds
// anywhere.
func knowledgeHours(db string) (counts [24]int, days int, err error) {
	rows, err := sqlite(db, hourCountsQuery)
	if err != nil {
		return counts, 0, err
	}
	for _, line := range rows {
		h, n, found := strings.Cut(line, "|")
		if !found {
			continue
		}
		hour, err1 := strconv.Atoi(strings.TrimSpace(h))
		count, err2 := strconv.Atoi(strings.TrimSpace(n))
		if err1 != nil || err2 != nil || hour < 0 || hour > 23 {
			continue
		}
		counts[hour] = count
	}

	out, err := sqlite(db, dayCountQuery)
	if err != nil {
		return counts, 0, err
	}
	if len(out) > 0 {
		days, _ = strconv.Atoi(strings.TrimSpace(out[0]))
	}
	return counts, days, nil
}

func sqlite(db, query string) ([]string, error) {
	out, err := exec.Command("sqlite3", db, query).Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			return nil, fmt.Errorf("sqlite3: %s", strings.TrimSpace(string(ee.Stderr)))
		}
		return nil, fmt.Errorf("sqlite3: %w", err)
	}
	return strings.Split(strings.TrimSpace(string(out)), "\n"), nil
}
