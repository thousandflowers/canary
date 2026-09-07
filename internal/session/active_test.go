package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func at(s ...int64) []int64 { return s }

func TestActiveMinutesCountsShortGapsAndSkipsBreaks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "active")
	const idle = 300

	// First sighting: the ledger opens at the newest entry with nothing credited.
	if got := ActiveMinutes(path, "s1", at(1000), 1000, idle); got != 0 {
		t.Fatalf("first refresh = %dm, want 0", got)
	}
	// Two minutes of transcript progress is two minutes of work.
	if got := ActiveMinutes(path, "s1", at(1000, 1120), 1120, idle); got != 2 {
		t.Errorf("after a 120s gap = %dm, want 2", got)
	}
	// A gap past the threshold is a break: the clock does not move.
	if got := ActiveMinutes(path, "s1", at(1000, 1120, 1120+3600), 1120+3600, idle); got != 2 {
		t.Errorf("after an hour away = %dm, want 2 still", got)
	}
	// Work resumes and counts again.
	if got := ActiveMinutes(path, "s1", at(1120+3600, 1120+3600+240), 1120+3600+240, idle); got != 6 {
		t.Errorf("after four more minutes = %dm, want 6", got)
	}
}

func TestABurstBetweenRefreshesCountsAsTheMinutesItTook(t *testing.T) {
	// One refresh saw 10:00. The next sees 14:00 and 14:02 at once: four hours
	// of nothing, then two minutes of work. Reading only the newest entry
	// would have seen one four-hour gap and credited nothing.
	path := filepath.Join(t.TempDir(), "active")
	ActiveMinutes(path, "s1", at(36000), 36000, 300)
	if got := ActiveMinutes(path, "s1", at(36000, 50400, 50520), 50520, 300); got != 2 {
		t.Errorf("burst = %dm, want 2", got)
	}
	// A session met mid-way is credited for what the window shows of it.
	if got := ActiveMinutes(path, "resumed", at(100, 160, 220, 4000), 4000, 300); got != 2 {
		t.Errorf("resumed session = %dm, want 2 from the window", got)
	}
}

func TestAStampFromTheFutureIsNotWalked(t *testing.T) {
	// A tool result quoting "timestamp":"2099-..." must not park the ledger
	// past every real entry. The real ones around it still count.
	path := filepath.Join(t.TempDir(), "active")
	ActiveMinutes(path, "s1", at(1000), 1000, 300)
	if got := ActiveMinutes(path, "s1", at(1000, 1060, 4_000_000_000, 1120), 1120, 300); got != 2 {
		t.Errorf("with a future stamp in the middle = %dm, want 2", got)
	}
	// And the walk resumes from the last real entry, not the fake one.
	if got := ActiveMinutes(path, "s1", at(1120, 1180), 1180, 300); got != 3 {
		t.Errorf("after the fake = %dm, want 3", got)
	}
}

func TestActiveMinutesRepaintsCreditNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "active")
	ActiveMinutes(path, "s1", at(1000), 1000, 300)
	ActiveMinutes(path, "s1", at(1000, 1060), 1060, 300)
	before, _ := os.ReadFile(path)
	// The row redraws all night; the transcript does not move.
	for i := int64(1); i <= 1000; i++ {
		if got := ActiveMinutes(path, "s1", at(1000, 1060), 1060+i*30, 300); got != 1 {
			t.Fatalf("repaint %d changed the minutes to %d", i, got)
		}
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Error("a repaint rewrote the ledger")
	}
}

func TestActiveMinutesWithoutATimestampOrASessionIsUnknown(t *testing.T) {
	path := filepath.Join(t.TempDir(), "active")
	if got := ActiveMinutes(path, "s1", nil, 1000, 300); got != -1 {
		t.Errorf("no timestamp = %d, want -1", got)
	}
	if got := ActiveMinutes(path, "", at(1000), 1000, 300); got != -1 {
		t.Errorf("no session id = %d, want -1", got)
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("an unknown answer still wrote a ledger")
	}
}

func TestLedgerKeepsSessionsApartAndForgetsOldOnes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "active")
	ActiveMinutes(path, "old", at(1000), 1000, 300)
	ActiveMinutes(path, "old", at(1000, 1100), 1100, 300)
	// A new session, three days later: its own count, and the old one is pruned.
	later := int64(1100 + 3*86400)
	if got := ActiveMinutes(path, "new", at(later), later, 300); got != 0 {
		t.Errorf("new session = %dm, want 0", got)
	}
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), "old ") {
		t.Errorf("a session three days quiet is still in the ledger:\n%s", raw)
	}
	if !strings.Contains(string(raw), "new ") {
		t.Errorf("the live session is missing from the ledger:\n%s", raw)
	}

	// A session resumed after a week is stale until its first new turn, and
	// is remembered anyway: forgetting it would mean a write on every refresh.
	stale := int64(later + 7*86400)
	ActiveMinutes(path, "resumed", at(1000, 1060), stale, 300)
	raw, _ = os.ReadFile(path)
	if !strings.Contains(string(raw), "resumed 1060 60") {
		t.Errorf("the session being recorded was pruned:\n%s", raw)
	}
	before := string(raw)
	ActiveMinutes(path, "resumed", at(1000, 1060), stale+30, 300)
	if after, _ := os.ReadFile(path); string(after) != before {
		t.Error("a repaint of a stale session rewrote the ledger")
	}
}

func TestLedgerSurvivesWhatItCannotRead(t *testing.T) {
	dir := t.TempDir()
	corrupt := filepath.Join(dir, "corrupt")
	os.WriteFile(corrupt, []byte("s1 notanumber 5\ns1 5 notanumber\ntoo few\ns1 1000 120\n"), 0o644)
	if got := ActiveMinutes(corrupt, "s1", at(1060), 1060, 300); got != 3 {
		t.Errorf("the one good line should survive: %dm, want 3", got)
	}

	link := filepath.Join(dir, "link")
	os.Symlink(corrupt, link)
	if got := ActiveMinutes(link, "s1", at(1000), 1000, 300); got != 0 {
		t.Errorf("a symlink was followed: %dm", got)
	}

	if os.Getuid() == 0 {
		t.Skip("root reads everything")
	}
	unreadable := filepath.Join(dir, "unreadable")
	os.WriteFile(unreadable, []byte("s1 1000 120\n"), 0o000)
	if got := ActiveMinutes(unreadable, "s1", at(1060), 1060, 300); got != 0 {
		t.Errorf("an unreadable ledger was read: %dm", got)
	}
}
