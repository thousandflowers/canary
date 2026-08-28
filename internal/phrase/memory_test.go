package phrase

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMemoryRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "phrase-state")
	want := Memory{Band: "worn", Score: 85, TS: 1_700_000_000, PhTS: 1_699_999_940, Phrase: "sliding. slowly.", Known: true}

	if err := SaveMemory(path, want); err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}
	if got := LoadMemory(path); got != want {
		t.Errorf("round trip: got %+v, want %+v", got, want)
	}
}

func TestMissingMemoryIsNotKnown(t *testing.T) {
	// The difference between "no previous score" and "a previous score of 0" is
	// the difference between an unknown trend and a steady one.
	got := LoadMemory(filepath.Join(t.TempDir(), "absent"))
	if got.Known {
		t.Error("a missing file reported a known score")
	}
}

func TestMemoryRefusesToCarryAnEscape(t *testing.T) {
	// The phrase comes back out onto a status row. Whatever is in this file,
	// what leaves it is printable text.
	path := filepath.Join(t.TempDir(), "phrase-state")
	os.WriteFile(path, []byte("state=wo!!rn\nscore=8\x1b[31m5\nph=hello\x1b[31mthere\n"), 0o644)

	got := LoadMemory(path)
	if got.Band != "worn" {
		t.Errorf("band = %q, want the letters only", got.Band)
	}
	if got.Known {
		t.Errorf("a score with an escape in it is not a score, got %d", got.Score)
	}
	if strings.ContainsRune(got.Phrase, 0x1b) {
		t.Errorf("an escape survived into the phrase: %q", got.Phrase)
	}
}

func TestSymlinkedMemoryIsRefused(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	link := filepath.Join(dir, "phrase-state")
	os.WriteFile(target, []byte("state=fresh\n"), 0o644)
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if got := LoadMemory(link); got.Band != "" {
		t.Error("LoadMemory followed a symlink")
	}
	if err := SaveMemory(link, Memory{Band: "dead"}); err != nil {
		t.Fatalf("SaveMemory should decline quietly, got %v", err)
	}
	if b, _ := os.ReadFile(target); string(b) != "state=fresh\n" {
		t.Errorf("SaveMemory wrote through a symlink: %q", b)
	}
}

func TestRecentKeepsTheLastFew(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recent")
	for i := 0; i < RecentKeep+5; i++ {
		if err := AppendRecent(path, string(rune('a'+i))); err != nil {
			t.Fatalf("AppendRecent: %v", err)
		}
	}
	got := LoadRecent(path)
	if len(got) != RecentKeep {
		t.Fatalf("recent holds %d lines, want %d", len(got), RecentKeep)
	}
	if got[len(got)-1] != string(rune('a'+RecentKeep+4)) {
		t.Errorf("the newest line was pruned: %q", got)
	}
}

func TestAppendRecentIgnoresSilence(t *testing.T) {
	// Silence is the common case. Recording it would flush the queue that
	// exists to stop a line repeating.
	path := filepath.Join(t.TempDir(), "recent")
	AppendRecent(path, "a line")
	AppendRecent(path, "")
	if got := LoadRecent(path); len(got) != 1 {
		t.Errorf("silence was recorded: %q", got)
	}
}

func TestNoTempFilesAreLeftBehind(t *testing.T) {
	// The shell left `*.tmp.$$` litter in ~/.canary every time Claude Code
	// killed it between the write and the rename.
	dir := t.TempDir()
	if err := SaveMemory(filepath.Join(dir, "phrase-state"), Memory{Band: "fresh"}); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp.") {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}

func TestAnUnreadableMemoryIsAnEmptyOne(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root; the mode would be ignored")
	}
	path := filepath.Join(t.TempDir(), "phrase-state")
	os.WriteFile(path, []byte("state=worn\n"), 0o000)
	if got := LoadMemory(path); got.Band != "" {
		t.Errorf("read %+v from an unreadable file", got)
	}
}

func TestMemoryIgnoresLinesThatAreNotFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "phrase-state")
	os.WriteFile(path, []byte("a line with no equals sign\nstate=tired\n"), 0o644)
	if got := LoadMemory(path); got.Band != "tired" {
		t.Errorf("a stray line broke the read: %+v", got)
	}
}

func TestTheUntranslatedLineIsSpentPerSession(t *testing.T) {
	path := filepath.Join(t.TempDir(), "phrase-state")
	if err := SaveMemory(path, Memory{Band: "fresh", MineSession: "abc-123"}); err != nil {
		t.Fatal(err)
	}
	if got := LoadMemory(path).MineSession; got != "abc-123" {
		t.Errorf("mine session = %q", got)
	}
	// And whatever else is in that field, what comes back is an id.
	os.WriteFile(path, []byte("mine_session=abc \x1b[31m/../evil\n"), 0o644)
	// The escape's introducer, the brackets and the path traversal are gone;
	// what is left is the character class a session id is made of.
	if got := LoadMemory(path).MineSession; got != "abc31mevil" {
		t.Errorf("mine session = %q, want the id characters only", got)
	}
}

func TestNegativeTimestampsAreNotTimestamps(t *testing.T) {
	path := filepath.Join(t.TempDir(), "phrase-state")
	os.WriteFile(path, []byte("ts=-99\nph_ts=-1\n"), 0o644)
	if got := LoadMemory(path); got.TS != 0 || got.PhTS != 0 {
		t.Errorf("negative times were kept: %+v", got)
	}
}

func TestRecentSurvivesAnUnwritableDirectory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root; the mode would be ignored")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Skip(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o755) })

	if err := AppendRecent(filepath.Join(dir, "recent"), "a line"); err == nil {
		t.Error("a failed write should be reported to the caller")
	}
}
