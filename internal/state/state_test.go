package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRecordAccruesOnlyActiveTime(t *testing.T) {
	// Arrange: a session whose last command was 10 seconds ago, and one whose
	// last command was an hour ago — a lunch break with the terminal open.
	s := State{LastActive: 1000}

	// Act
	near := s.Record("git status", 1010, 300)
	far := s.Record("git status", 1000+3600, 300)

	// Assert
	if near.ActiveSeconds != 10 {
		t.Errorf("a 10s gap should accrue 10s, got %d", near.ActiveSeconds)
	}
	if far.ActiveSeconds != 0 {
		t.Errorf("an hour idle is a break, not work; accrued %d", far.ActiveSeconds)
	}
	if far.LastActive != 1000+3600 {
		t.Errorf("the clock still moves on a break: LastActive=%d", far.LastActive)
	}
}

func TestRecordDoesNotMutateTheReceiver(t *testing.T) {
	s := State{PromptCount: 2, LenSum: 10, LastActive: 1000}
	_ = s.Record("ls", 1005, 300)
	if s.PromptCount != 2 || s.LenSum != 10 {
		t.Errorf("Record mutated its receiver: %+v", s)
	}
}

func TestFirstCommandAccruesNothing(t *testing.T) {
	// A fresh state has no previous command, so there is no gap to charge for —
	// otherwise the very first command of the day would bill the whole epoch.
	s := State{}.Record("ls", 1_700_000_000, 300)
	if s.ActiveSeconds != 0 {
		t.Errorf("first command accrued %d seconds", s.ActiveSeconds)
	}
	if s.PromptCount != 1 {
		t.Errorf("first command not counted: %d", s.PromptCount)
	}
}

func TestLenSumCountsCharactersNotBytes(t *testing.T) {
	// bash's ${#var} counts characters in a UTF-8 locale, and canary requires a
	// UTF-8 terminal. Counting bytes would score an accented command as longer.
	s := State{}.Record("echo àèìòù", 1000, 300) // 10 characters, 15 bytes
	if s.LenSum != 10 {
		t.Errorf("LenSum = %d, want 10 characters", s.LenSum)
	}
}

func TestAvgLenIsZeroOnAnEmptySession(t *testing.T) {
	if got := (State{}).AvgLen(); got != 0 {
		t.Errorf("AvgLen on no commands = %d, want 0 (not a division by zero)", got)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "canary-state")
	want := State{PromptCount: 7, LenSum: 140, ActiveSeconds: 3600, LastActive: 1_700_000_000}

	if err := Save(path, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, ok := Load(path)
	if !ok {
		t.Fatal("Load reported no usable state after Save")
	}
	if got != want {
		t.Errorf("round trip: got %+v, want %+v", got, want)
	}
}

func TestSaveKeepsTheKeysTheShellBirdWrote(t *testing.T) {
	// An older canary-statusline.sh still reads this file. Dropping the derived
	// average would leave it computing a score with avg_prompt_len=0.
	path := filepath.Join(t.TempDir(), "canary-state")
	if err := Save(path, State{PromptCount: 4, LenSum: 40}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"prompt_count=4", "avg_prompt_len=10", "active_seconds=0"} {
		if !contains(string(b), key) {
			t.Errorf("state file is missing %q:\n%s", key, b)
		}
	}
}

func TestLoadRebuildsLenSumFromAShellWrittenFile(t *testing.T) {
	// The upgrade path: a file written by canary.sh has an average but no sum.
	// Without this the session's average resets the moment the binary takes over.
	path := filepath.Join(t.TempDir(), "canary-state")
	os.WriteFile(path, []byte("prompt_count=4\navg_prompt_len=25\nactive_seconds=600\n"), 0o644)

	got, ok := Load(path)
	if !ok {
		t.Fatal("Load refused a shell-written state file")
	}
	if got.LenSum != 100 || got.AvgLen() != 25 {
		t.Errorf("LenSum=%d AvgLen=%d, want 100 and 25", got.LenSum, got.AvgLen())
	}
}

func TestLoadSurvivesGarbage(t *testing.T) {
	// This file is data on disk. A corrupt line costs that line, nothing more,
	// and nothing in it gets to reach a prompt.
	path := filepath.Join(t.TempDir(), "canary-state")
	os.WriteFile(path, []byte("prompt_count=\x1b[31m9\nnonsense\nactive_seconds=abc\n"), 0o644)

	got, ok := Load(path)
	if !ok {
		t.Fatal("Load gave up on a partly corrupt file")
	}
	// Not 319. The shell's `tr -cd '0-9'` kept the digits inside the escape
	// sequence and handed back a number nobody wrote.
	if got.PromptCount != 0 {
		t.Errorf("PromptCount = %d, want 0: a field with an escape in it is not a number", got.PromptCount)
	}
	if got.ActiveSeconds != 0 {
		t.Errorf("a non-numeric field should be skipped, got %d", got.ActiveSeconds)
	}
}

func TestSymlinkedStateIsRefused(t *testing.T) {
	// Anything that can write to ~/.canary could otherwise redirect a write
	// that happens on every command.
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	link := filepath.Join(dir, "canary-state")
	os.WriteFile(target, []byte("prompt_count=5\n"), 0o644)
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if _, ok := Load(link); ok {
		t.Error("Load followed a symlink")
	}
	if err := Save(link, State{PromptCount: 99}); err != nil {
		t.Fatalf("Save should decline quietly, got %v", err)
	}
	b, _ := os.ReadFile(target)
	if string(b) != "prompt_count=5\n" {
		t.Errorf("Save wrote through a symlink: %q", b)
	}
}

func contains(hay, needle string) bool {
	return len(hay) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(hay); i++ {
			if hay[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})()
}

func TestLoadRefusesAPathItCannotStat(t *testing.T) {
	// A file where a directory should be.
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	os.WriteFile(blocker, nil, 0o644)
	if _, ok := Load(filepath.Join(blocker, "canary-state")); ok {
		t.Error("a path under a regular file reported usable state")
	}
}

func TestAnUnreadableStateFileIsNoState(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root; the mode would be ignored")
	}
	path := filepath.Join(t.TempDir(), "canary-state")
	os.WriteFile(path, []byte("prompt_count=3\n"), 0o000)
	if _, ok := Load(path); ok {
		t.Error("an unreadable state file reported usable state")
	}
}

func TestSaveReportsAWriteItCannotMake(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root; the mode would be ignored")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Skip(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o755) })

	if err := Save(filepath.Join(dir, "canary-state"), State{}); err == nil {
		t.Error("a failed write should be reported to the caller")
	}
}

func TestTheUpgradeReadIsAsCarefulAsTheOrdinaryOne(t *testing.T) {
	dir := t.TempDir()
	// No file at all: nothing to reconstruct from.
	if got := readInt(filepath.Join(dir, "absent"), "avg_prompt_len"); got != 0 {
		t.Errorf("got %d from a missing file", got)
	}
	// A file without the key in it.
	path := filepath.Join(dir, "canary-state")
	os.WriteFile(path, []byte("prompt_count=4\n"), 0o644)
	if got := readInt(path, "avg_prompt_len"); got != 0 {
		t.Errorf("got %d for a key that is not there", got)
	}
	// And a value that is not a number.
	os.WriteFile(path, []byte("avg_prompt_len=lots\n"), 0o644)
	if got := readInt(path, "avg_prompt_len"); got != 0 {
		t.Errorf("got %d for a value that is not a number", got)
	}
}

func TestParseIntAcceptsAPlainNumberAndNothingElse(t *testing.T) {
	// The last one is all digits and still not a number: it does not fit.
	for _, s := range []string{"", "   ", "12a", "-3", "1.5", "99999999999999999999999999"} {
		if _, ok := parseInt(s); ok {
			t.Errorf("%q was accepted as a number", s)
		}
	}
	if n, ok := parseInt(" 7 "); !ok || n != 7 {
		t.Errorf("parseInt(\" 7 \") = %d, %v", n, ok)
	}
}
