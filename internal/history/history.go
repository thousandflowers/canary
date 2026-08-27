// Package history reads and writes the daily fatigue peaks that let the bird
// remember yesterday.
//
// The file is one "<epoch-day> <peak>" per line, pruned to the last 10 days.
// The format is shared with the shell birds and must stay readable by them for
// as long as both exist side by side.
package history

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Keep is how many days of peaks survive a prune.
const Keep = 10

// Entry is one day's raw peak.
type Entry struct {
	Day  int // epoch day: unix seconds / 86400
	Peak int // the day's highest RAW score, before debt was added
}

// Summary is what the past contributes to today.
type Summary struct {
	// Debt is prior peaks decayed by half per day of age, summed and capped.
	// It recovers over ~4-5 days.
	Debt int
	// Personal is the mean of prior peaks — the person's own baseline, which
	// is what the dead-bird demotion compares today against.
	Personal int
	// Nights counts consecutive days ending yesterday with a peak >= 90.
	Nights int
}

// Load parses a history file. A missing file is not an error: a first run has
// no past, and the bird should start fresh rather than refuse to draw.
//
// Symlinks are refused. This path is read and rewritten on every statusline
// refresh, and following a link would let anything that can write to
// ~/.canary redirect that write somewhere else.
func Load(path string) ([]Entry, error) {
	if fi, err := os.Lstat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	} else if fi.Mode()&os.ModeSymlink != 0 {
		return nil, nil
	}

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var out []Entry
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 2 {
			continue
		}
		// awk's $1+0 / $2+0 coerce junk to 0 rather than failing. A corrupt
		// line must not take the bird down, so a bad field is skipped.
		day, err1 := strconv.Atoi(fields[0])
		peak, err2 := strconv.Atoi(fields[1])
		if err1 != nil || err2 != nil {
			continue
		}
		out = append(out, Entry{Day: day, Peak: peak})
	}
	return out, sc.Err()
}

// Summarize folds prior days into today's debt, baseline and night streak.
//
// The halving is deliberately float arithmetic, exactly as the awk it replaces:
// a peak of 95 two days old contributes 23.75, and three such days sum to more
// than truncating each one first would give. Rounding per-day instead of once
// at the end shifts the debt by whole points, which is enough to move a band.
func Summarize(entries []Entry, today, debtMax int) Summary {
	var debt float64
	var peakSum, peakCount int

	for _, e := range entries {
		if e.Day >= today {
			continue // today is not its own debt; it is the thing being scored
		}
		age := today - e.Day
		v := float64(e.Peak)
		for k := 0; k < age; k++ {
			v /= 2
		}
		debt += v
		peakSum += e.Peak
		peakCount++
	}

	// The cap lands on the float, before truncation, matching the awk.
	if debt > float64(debtMax) {
		debt = float64(debtMax)
	}

	personal := 0
	if peakCount > 0 {
		personal = peakSum / peakCount
	}

	return Summary{
		Debt:     int(debt),
		Personal: personal,
		Nights:   streak(entries, today),
	}
}

// streak walks back from yesterday while each day has a peak at or past 90.
// A gap ends it: the point is consecutive nights, and one recovered day is
// exactly the thing that should reset the count.
func streak(entries []Entry, today int) int {
	peaks := make(map[int]int, len(entries))
	for _, e := range entries {
		// Last writer wins, as the awk's linear scan did.
		if p, ok := peaks[e.Day]; !ok || e.Peak > p {
			peaks[e.Day] = e.Peak
		}
	}
	n := 0
	for day := today - 1; ; day-- {
		if p, ok := peaks[day]; !ok || p < 90 {
			return n
		}
		n++
	}
}

// RecordPeak stores today's RAW peak — pre-debt, so yesterday's debt never
// compounds into tomorrow's — and prunes the file to the last Keep days.
//
// The write is atomic via a temp file in the same directory: Claude Code
// cancels this process mid-run routinely, and a half-written history is a
// corrupt one. The shell left `history.tmp.$$` files behind when it was killed
// between write and rename; a deferred cleanup closes that leak.
func RecordPeak(path string, today, raw int) error {
	if fi, err := os.Lstat(path); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		return nil // refuse to follow a link, same as Load
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	entries, err := Load(path)
	if err != nil {
		return err
	}

	// Merge today: keep the highest peak seen so far, and note whether the file
	// already knew about today at all.
	merged := make([]Entry, 0, len(entries)+1)
	found := false
	for _, e := range entries {
		if e.Day == today {
			if !found {
				found = true
				if e.Peak > raw {
					raw = e.Peak
				}
			}
			continue
		}
		merged = append(merged, e)
	}
	merged = append(merged, Entry{Day: today, Peak: raw})

	sort.SliceStable(merged, func(i, j int) bool { return merged[i].Day < merged[j].Day })
	if len(merged) > Keep {
		merged = merged[len(merged)-Keep:]
	}

	var b strings.Builder
	for _, e := range merged {
		fmt.Fprintf(&b, "%d %d\n", e.Day, e.Peak)
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp.*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	// If anything below fails, do not leave the temp file behind.
	defer os.Remove(tmpName)

	if _, err := tmp.WriteString(b.String()); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
