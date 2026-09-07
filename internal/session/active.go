package session

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/thousandflowers/canary/internal/atomicfile"
)

// ledgerKeep is how long a session stays in the ledger after its last entry.
// Two days covers a session resumed the next morning; anything older is a
// transcript nobody is reading any more.
const ledgerKeep = 2 * 86400

// futureSlack is how far ahead of the clock a transcript entry may be and
// still count: clocks disagree by seconds, not by hours.
const futureSlack = 120

type ledgerEntry struct{ last, active int64 }

// ActiveMinutes is the Claude Code half of "a gap longer than five minutes is
// a break". cost.total_duration_ms is wall-clock: a window left open all
// afternoon would age the bird as if you had typed all afternoon. The
// transcript is the record of activity instead, but only its tail is read, so
// this keeps a small ledger per session: the newest transcript entry seen and
// the seconds of activity credited so far. Each refresh, the transcript having
// moved on since the last one counts as work when the gap is short and as a
// break when it is not — the rule the shell bird's state file applies to
// commands, applied to turns and tool calls.
//
// A refresh on its own credits nothing. Claude Code repaints the row all night
// with nobody at the keyboard, and a repaint is not a person.
//
// Two sessions refreshing at once can race on the file, and the loser's write
// is lost. That is survivable without a lock: what is stored is where the
// walk got to, so the next refresh walks the same entries again and lands on
// the same count. A stamp from the future is not walked at all — one pasted
// into a tool result would otherwise park the ledger past every real entry
// and credit nothing for two days.
//
// stamps are the unix seconds of the transcript entries in the tail, in
// order. Every entry newer than the last one credited is walked, so a burst
// of tool calls between two refreshes counts as the minutes it took, not as
// one jump. No stamps means the transcript carried no timestamps, and the
// answer is -1: the wall clock is all there is.
func ActiveMinutes(path, id string, stamps []int64, now int64, idle int) int {
	if len(stamps) == 0 || id == "" {
		return -1
	}
	entries := loadLedger(path)
	cur, seen := entries[id]
	if seen && stamps[len(stamps)-1] <= cur.last {
		return int(cur.active / 60) // nothing new: no write on the hot path
	}
	// A session first met mid-way starts from the oldest entry in the window,
	// which is the most that can be known about it.
	prev := cur.last
	if !seen {
		prev = stamps[0]
	}
	for _, s := range stamps {
		if s <= prev || s > now+futureSlack {
			continue
		}
		if gap := s - prev; gap <= int64(idle) {
			cur.active += gap
		}
		prev = s
	}
	cur.last = prev
	entries[id] = cur
	saveLedger(path, entries, now, id)
	return int(cur.active / 60)
}

// loadLedger reads `id last active` lines. Anything it cannot read is an
// empty ledger, and a symlink is refused like every other file under ~/.canary.
func loadLedger(path string) map[string]ledgerEntry {
	out := map[string]ledgerEntry{}
	fi, err := os.Lstat(path)
	if err != nil || fi.Mode()&os.ModeSymlink != 0 || !fi.Mode().IsRegular() {
		return out
	}
	f, err := os.Open(path)
	if err != nil {
		return out
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fs := strings.Fields(sc.Text())
		if len(fs) != 3 {
			continue
		}
		last, err1 := strconv.ParseInt(fs[1], 10, 64)
		active, err2 := strconv.ParseInt(fs[2], 10, 64)
		if err1 != nil || err2 != nil {
			continue
		}
		out[fs[0]] = ledgerEntry{last, active}
	}
	return out
}

// saveLedger writes the sessions still worth remembering, oldest dropped —
// never the one being recorded, however old its transcript: a session resumed
// after a week is stale until its first new turn, and forgetting it on every
// refresh until then would mean a write on every refresh. Errors are dropped
// as everywhere on this path: a minute lost from a status row is not worth a
// line of noise in it.
func saveLedger(path string, entries map[string]ledgerEntry, now int64, keep string) {
	ids := make([]string, 0, len(entries))
	for id, e := range entries {
		if id == keep || now-e.last <= ledgerKeep {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	var b strings.Builder
	for _, id := range ids {
		e := entries[id]
		b.WriteString(id + " " + strconv.FormatInt(e.last, 10) + " " + strconv.FormatInt(e.active, 10) + "\n")
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	_ = atomicfile.Write(path, []byte(b.String()))
}
