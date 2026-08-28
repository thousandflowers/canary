package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// transcript writes a JSONL file and returns the Claude Code payload pointing
// at it, which is the only shape this package is ever handed in production.
func transcript(t *testing.T, lines ...string) (string, []byte) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "transcript.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path, []byte(fmt.Sprintf(`{"cost":{"total_duration_ms":3600000},"transcript_path":%q}`, path))
}

func TestIsClaudeCodeDiscriminatesOnTheTranscriptPath(t *testing.T) {
	if !IsClaudeCode([]byte(`{"transcript_path":"/tmp/x.jsonl"}`)) {
		t.Error("Claude Code's own payload was not recognised")
	}
	for _, other := range []string{"", "{}", "prompt_count=3"} {
		if IsClaudeCode([]byte(other)) {
			t.Errorf("%q was read as a Claude Code session", other)
		}
	}
}

func TestClaudeCodeCountsHumanTurnsNotToolResults(t *testing.T) {
	// Claude Code wraps every tool result as its own "type":"user" line.
	// Subtracting one count from the other used to underflow to zero.
	_, in := transcript(t,
		`{"type":"user","message":{"content":"fix the build"}}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","is_error":true}]}}`,
		`{"type":"assistant","message":{"content":"on it"}}`,
		`{"type":"user","message":{"content":"still broken?"}}`,
	)
	got := FromClaudeCode(in)

	if got.Turns != 2 {
		t.Errorf("turns = %d, want 2 human turns", got.Turns)
	}
	if got.Errors != 1 {
		t.Errorf("errors = %d, want 1", got.Errors)
	}
	if got.Minutes != 60 {
		t.Errorf("minutes = %d, want 60 from total_duration_ms", got.Minutes)
	}
	if got.StatName != "t" {
		t.Errorf("stat label = %q, want t for turns", got.StatName)
	}
}

func TestRepsCountTheLongestRunOfOneCommand(t *testing.T) {
	// Perseveration: repeating an action that is not working. A run of three is
	// two repeats.
	_, in := transcript(t,
		`{"command":"go build ./..."}`,
		`{"command":"go build ./..."}`,
		`{"command":"go build ./..."}`,
		`{"command":"ls"}`,
		`{"command":"go build ./..."}`,
	)
	if got := FromClaudeCode(in).Reps; got != 2 {
		t.Errorf("reps = %d, want 2", got)
	}
}

func TestRepsAreCappedSoAPollingLoopCannotKillTheBird(t *testing.T) {
	lines := make([]string, 0, 40)
	for i := 0; i < 40; i++ {
		lines = append(lines, `{"command":"sleep 1"}`)
	}
	_, in := transcript(t, lines...)
	if got := FromClaudeCode(in).Reps; got != MaxReps {
		t.Errorf("reps = %d, want the cap %d", got, MaxReps)
	}
}

func TestAMalformedPayloadIsQuietNotFatal(t *testing.T) {
	// This runs on every refresh. A bad frame makes the bird quiet; it must
	// never crash into Claude Code's status row.
	got := FromClaudeCode([]byte(`{"transcript_path": not json`))
	if got != (Signals{StatName: "t"}) {
		t.Errorf("a malformed payload produced signals: %+v", got)
	}
}

func TestAMissingTranscriptStillGivesTheMinutes(t *testing.T) {
	in := []byte(`{"cost":{"total_duration_ms":600000},"transcript_path":"/nonexistent/x.jsonl"}`)
	got := FromClaudeCode(in)
	if got.Minutes != 10 {
		t.Errorf("minutes = %d, want 10 even with no transcript", got.Minutes)
	}
	if got.Turns != 0 {
		t.Errorf("turns = %d, want 0 with nothing to read", got.Turns)
	}
}

func TestASymlinkedTranscriptIsRefused(t *testing.T) {
	// The path arrives from outside the process.
	dir := t.TempDir()
	real := filepath.Join(dir, "real.jsonl")
	os.WriteFile(real, []byte(`{"type":"user","message":{"content":"hi"}}`+"\n"), 0o644)
	link := filepath.Join(dir, "link.jsonl")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	in := []byte(fmt.Sprintf(`{"cost":{"total_duration_ms":0},"transcript_path":%q}`, link))
	if got := FromClaudeCode(in).Turns; got != 0 {
		t.Errorf("a symlinked transcript was read: turns = %d", got)
	}
}

func TestOnlyTheTailOfAHugeTranscriptIsScanned(t *testing.T) {
	// Scanning tens of MB several times a refresh blew Claude Code's timeout,
	// so nothing rendered at all — the bird went invisible in exactly the long
	// sessions it exists to warn about.
	var b strings.Builder
	filler := `{"type":"assistant","message":{"content":"` + strings.Repeat("x", 4000) + `"}}`
	for b.Len() < TranscriptTail+(1<<20) {
		b.WriteString(filler + "\n")
	}
	early := `{"type":"user","message":{"content":"the first thing i asked"}}`
	path := filepath.Join(t.TempDir(), "huge.jsonl")
	os.WriteFile(path, []byte(early+"\n"+b.String()+`{"type":"user","message":{"content":"the last thing"}}`+"\n"), 0o644)

	in := []byte(fmt.Sprintf(`{"cost":{"total_duration_ms":0},"transcript_path":%q}`, path))
	if got := FromClaudeCode(in).Turns; got != 1 {
		t.Errorf("turns = %d, want only the tail's single turn", got)
	}
}

func TestShellStateFallback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "canary-state")
	os.WriteFile(path, []byte("prompt_count=12\navg_prompt_len=30\nactive_seconds=1800\n"), 0o644)

	got, ok := FromShellState(path)
	if !ok {
		t.Fatal("a valid state file was refused")
	}
	want := Signals{Minutes: 30, Turns: 12, AvgLen: 30, StatName: "p"}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestNoStateMeansSayNothing(t *testing.T) {
	// A statusline with no session and no state has nothing true to report, and
	// a fresh bird there would be a lie.
	if _, ok := FromShellState(filepath.Join(t.TempDir(), "absent")); ok {
		t.Error("a missing state file reported usable signals")
	}
}

func TestShellStateRejectsANonNumericField(t *testing.T) {
	path := filepath.Join(t.TempDir(), "canary-state")
	os.WriteFile(path, []byte("prompt_count=\x1b[31m9\nactive_seconds=600\n"), 0o644)

	got, ok := FromShellState(path)
	if !ok {
		t.Fatal("the file should still be readable")
	}
	if got.Turns != 0 {
		t.Errorf("turns = %d, want 0: digits squeezed out of an escape are not a count", got.Turns)
	}
	if got.Minutes != 10 {
		t.Errorf("the good field should survive: minutes = %d", got.Minutes)
	}
}

func TestATestFileInTheSessionIsNoticed(t *testing.T) {
	_, in := transcript(t,
		`{"type":"assistant","message":{"content":[{"type":"tool_use","input":{"file_path":"/repo/internal/state/state_test.go"}}]}}`,
	)
	if !FromClaudeCode(in).HasTests {
		t.Error("a test file was touched and not noticed")
	}
}

func TestADirectoryIsNotATranscript(t *testing.T) {
	dir := t.TempDir()
	in := []byte(fmt.Sprintf(`{"cost":{"total_duration_ms":0},"transcript_path":%q}`, dir))
	if got := FromClaudeCode(in).Turns; got != 0 {
		t.Errorf("read %d turns out of a directory", got)
	}
}

func TestOneLineLongerThanTheWholeWindowIsGivenUpOn(t *testing.T) {
	// The tail window opens mid-line and the fragment is dropped. If there is
	// no line break in the whole window there is nothing safe to count.
	path := filepath.Join(t.TempDir(), "huge.jsonl")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", TranscriptTail+(1<<20))), 0o644); err != nil {
		t.Fatal(err)
	}
	in := []byte(fmt.Sprintf(`{"cost":{"total_duration_ms":0},"transcript_path":%q}`, path))
	if got := FromClaudeCode(in).Turns; got != 0 {
		t.Errorf("counted %d turns in a file with no lines in it", got)
	}
}

func TestFirstAskHandlesWhatItIsGiven(t *testing.T) {
	long := strings.Repeat("a question about the parser ", 10)
	cases := []struct {
		name string
		line string
		want bool // does it produce a comparable ask
	}{
		{"no content at all", `{"type":"user"}`, false},
		{"too short to mean anything", `{"type":"user","message":{"content":"ok"}}`, false},
		{"an escaped quote inside", `{"type":"user","message":{"content":"the \"parser\" drops rows"}}`, true},
		// One backslash, not two: the escape skip walks off the end of the line,
		// which is the case that needs clamping.
		{"a trailing escape", `{"type":"user","message":{"content":"the parser drops rows\`, true},
		{"longer than the prefix", `{"type":"user","message":{"content":"` + long + `"}}`, true},
	}
	for _, c := range cases {
		got := firstAsk([]byte(c.line))
		if (got != "") != c.want {
			t.Errorf("%s: firstAsk = %q", c.name, got)
		}
		if len(got) > 80 {
			t.Errorf("%s: %d characters, want the prefix only", c.name, len(got))
		}
	}
}

func TestValuesOfWalksALineWithSeveralValues(t *testing.T) {
	line := []byte(`{"command":"one"},{"command":"two \" quoted"},{"command":"three"}`)
	got := valuesOf(line, `"command":"`)
	if len(got) != 3 || got[0] != "one" || got[2] != "three" {
		t.Errorf("got %q", got)
	}
	// A value that never closes ends the walk rather than reading past it.
	if got := valuesOf([]byte(`{"command":"unterminated`), `"command":"`); len(got) != 1 {
		t.Errorf("unterminated: %q", got)
	}
	// A single trailing backslash makes the escape skip walk off the end.
	if got := valuesOf([]byte(`{"command":"ends with a backslash\`), `"command":"`); len(got) != 0 {
		t.Errorf("trailing escape: %q", got)
	}
	if got := valuesOf([]byte(`nothing here`), `"command":"`); got != nil {
		t.Errorf("got %q from a line with no values", got)
	}
}

func TestAnUnreadableTranscriptIsNoTranscript(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root; the mode would be ignored")
	}
	path := filepath.Join(t.TempDir(), "transcript.jsonl")
	os.WriteFile(path, []byte(`{"type":"user","message":{"content":"hello"}}`+"\n"), 0o000)
	in := []byte(fmt.Sprintf(`{"cost":{"total_duration_ms":0},"transcript_path":%q}`, path))
	if got := FromClaudeCode(in).Turns; got != 0 {
		t.Errorf("read %d turns out of an unreadable file", got)
	}
}

func TestAnUnreadableStateFileIsNoState(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root; the mode would be ignored")
	}
	path := filepath.Join(t.TempDir(), "canary-state")
	os.WriteFile(path, []byte("prompt_count=3\n"), 0o000)
	if _, ok := FromShellState(path); ok {
		t.Error("an unreadable state file reported usable signals")
	}
}

func TestShellStateIgnoresLinesThatAreNotFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "canary-state")
	os.WriteFile(path, []byte("a stray line\nprompt_count=3\n"), 0o644)
	got, ok := FromShellState(path)
	if !ok || got.Turns != 3 {
		t.Errorf("got %+v ok=%v", got, ok)
	}
}

func TestParseIntAcceptsAPlainNumberAndNothingElse(t *testing.T) {
	// The last one is all digits and still not a number: it does not fit.
	for _, s := range []string{"", "  ", "12a", "-3", "1.5", "\x1b[31m9", "99999999999999999999999999"} {
		if _, ok := parseInt(s); ok {
			t.Errorf("%q was accepted as a number", s)
		}
	}
	if n, ok := parseInt(" 42 "); !ok || n != 42 {
		t.Errorf("parseInt(\" 42 \") = %d, %v", n, ok)
	}
}
