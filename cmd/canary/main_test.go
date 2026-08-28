package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// binary is the built canary, exercised the way a person exercises it: as a
// process, with an environment and a $HOME. The packages below it are tested in
// isolation; this file is about the wiring between them.
var binary string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "canary-e2e")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer os.RemoveAll(dir)

	binary = filepath.Join(dir, "canary")
	build := exec.Command("go", "build", "-o", binary, ".")
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build failed: %v\n%s", err, out)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

// runBinary executes the built binary in an isolated home and returns its
// output and exit code. Every test gets its own home: these commands write
// state.
//
// The in-process tests in unit_test.go cover the same code far more cheaply;
// what only a real process can prove is the wiring — that the binary someone
// installs parses its arguments, reads its stdin and exits with the right code.
func runBinary(t *testing.T, home string, env []string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Env = append([]string{
		"HOME=" + home,
		"PATH=" + os.Getenv("PATH"),
		"CLAUDE_CONFIG_DIR=" + filepath.Join(home, ".claude"),
		"COLUMNS=100",
	}, env...)
	cmd.Stdin = strings.NewReader("") // never a tty, so stdin is read and empty
	var out strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("running %v: %v", args, err)
	}
	return out.String(), code
}

func TestRecordThenScore(t *testing.T) {
	home := t.TempDir()
	for _, cmd := range []string{"git status", "make test", "vim internal/state/state.go"} {
		if _, code := runBinary(t, home, nil, "record", "--", cmd); code != 0 {
			t.Fatalf("record exited %d", code)
		}
	}

	out, code := runBinary(t, home, nil, "score")
	if code != 0 {
		t.Fatalf("score exited %d: %s", code, out)
	}
	if strings.TrimSpace(out) == "" {
		t.Fatal("score printed nothing")
	}

	state, err := os.ReadFile(filepath.Join(home, ".canary", "canary-state"))
	if err != nil {
		t.Fatalf("no state file: %v", err)
	}
	if !strings.Contains(string(state), "prompt_count=3") {
		t.Errorf("three commands were not counted:\n%s", state)
	}
}

func TestRecordIgnoresABareEnter(t *testing.T) {
	home := t.TempDir()
	runBinary(t, home, nil, "record", "--", "   ")
	if _, err := os.Stat(filepath.Join(home, ".canary", "canary-state")); err == nil {
		t.Error("an empty command line was recorded as work")
	}
}

func TestPromptStaysQuietBelowTheThreshold(t *testing.T) {
	home := t.TempDir()
	runBinary(t, home, nil, "record", "--", "ls")

	quiet, _ := runBinary(t, home, []string{"CANARY_MIN_SCORE=71"}, "prompt")
	if quiet != "" {
		t.Errorf("the bird drew below its threshold:\n%s", quiet)
	}
	loud, _ := runBinary(t, home, nil, "prompt")
	if !strings.Contains(loud, "▐ O ▌>") {
		t.Errorf("the bird did not draw:\n%s", loud)
	}
}

func TestDisabledSilencesTheHooksButNotAnExplicitAsk(t *testing.T) {
	home := t.TempDir()
	off := []string{"CANARY_DISABLED=1"}

	if out, _ := runBinary(t, home, off, "prompt"); out != "" {
		t.Errorf("prompt spoke while disabled:\n%s", out)
	}
	if out, _ := runBinary(t, home, off, "statusline"); out != "" {
		t.Errorf("statusline spoke while disabled:\n%s", out)
	}
	// Asking the bird directly is a different thing from it appearing uninvited.
	if out, _ := runBinary(t, home, off, "score"); strings.TrimSpace(out) == "" {
		t.Error("`canary score` refused to answer while disabled")
	}
}

func TestResetStartsTheSessionOver(t *testing.T) {
	home := t.TempDir()
	for i := 0; i < 5; i++ {
		runBinary(t, home, nil, "record", "--", "a long command line to raise the average")
	}
	if out, _ := runBinary(t, home, nil, "reset"); !strings.Contains(out, "canary: reset") {
		t.Errorf("reset said nothing:\n%s", out)
	}
	state, _ := os.ReadFile(filepath.Join(home, ".canary", "canary-state"))
	if !strings.Contains(string(state), "prompt_count=0") {
		t.Errorf("the count survived a reset:\n%s", state)
	}
}

func TestStatuslineReadsAClaudeCodeSession(t *testing.T) {
	home := t.TempDir()
	transcript := filepath.Join(home, "transcript.jsonl")
	os.WriteFile(transcript, []byte(strings.Join([]string{
		`{"type":"user","message":{"content":"why is this failing"}}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","is_error":true}]}}`,
		`{"type":"user","message":{"content":"and now?"}}`,
	}, "\n")+"\n"), 0o644)

	payload := fmt.Sprintf(`{"cost":{"total_duration_ms":10800000},"transcript_path":%q}`, transcript)
	cmd := exec.Command(binary, "statusline")
	cmd.Env = []string{"HOME=" + home, "PATH=" + os.Getenv("PATH"), "COLUMNS=100", "CANARY_SHOW_SCORE=1"}
	cmd.Stdin = strings.NewReader(payload)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("statusline: %v", err)
	}

	got := string(out)
	if !strings.Contains(got, "180m") {
		t.Errorf("three hours were not read from the session JSON:\n%s", got)
	}
	if !strings.Contains(got, "2t") {
		t.Errorf("human turns were miscounted:\n%s", got)
	}
	if strings.HasSuffix(got, "\n") {
		t.Errorf("a trailing newline would push the next status segment onto its own row:\n%q", got)
	}
	// The corpus is in the binary: there is no ~/.canary/phrases here, and the
	// bird must still be able to speak. Shipping mute was the v0.7.0 bug.
	if !strings.Contains(got, "⌐") && !strings.Contains(got, "♪") {
		t.Errorf("neither a phrase nor a note reached the slot:\n%s", got)
	}
}

func TestStatuslineSaysNothingWithNoSessionAndNoState(t *testing.T) {
	// A fresh bird there would be a lie.
	if out, code := runBinary(t, t.TempDir(), nil, "statusline"); out != "" || code != 0 {
		t.Errorf("statusline invented a session: %q (exit %d)", out, code)
	}
}

func TestStatuslineFallsBackToTheShellState(t *testing.T) {
	home := t.TempDir()
	for i := 0; i < 4; i++ {
		runBinary(t, home, nil, "record", "--", "go test ./...")
	}
	out, _ := runBinary(t, home, nil, "statusline")
	if !strings.Contains(out, "4p") {
		t.Errorf("the shell's prompts did not reach the status row:\n%s", out)
	}
}

func TestPreviewDrawsAnyStateOnDemand(t *testing.T) {
	home := t.TempDir()
	out, code := runBinary(t, home, nil, "preview", "--state", "worn", "--note", "falling")
	if code != 0 {
		t.Fatalf("preview exited %d: %s", code, out)
	}
	if !strings.Contains(out, "▗▓▓▓▖") || !strings.Contains(out, "▐ ~ ▌>") {
		t.Errorf("preview drew the wrong bird:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(home, ".canary")); err == nil {
		t.Error("preview wrote state; it must not")
	}

	// Not just the art: preview has to reach the corpus. It silently stopped
	// doing so once, when the phrase paths gained a language prefix, and the
	// art-only assertion above did not notice.
	dead, _ := runBinary(t, home, nil, "preview", "--state", "dead")
	if !strings.Contains(dead, "the canary is quiet.") {
		t.Errorf("preview drew no line from the corpus:\n%s", dead)
	}

	custom, _ := runBinary(t, home, nil, "preview", "--state", "fresh", "--phrase", "a candidate line")
	if !strings.Contains(custom, "a candidate line") {
		t.Errorf("a contributor's own line was not drawn:\n%s", custom)
	}
	if _, code := runBinary(t, home, nil, "preview", "--state", "exhausted"); code != 2 {
		t.Error("an unknown state should be a usage error")
	}
}

func TestInitPrintsAHookForEveryShellItClaims(t *testing.T) {
	for _, shell := range []string{"zsh", "bash", "fish"} {
		out, code := runBinary(t, t.TempDir(), nil, "init", shell)
		if code != 0 {
			t.Fatalf("init %s exited %d: %s", shell, code, out)
		}
		// The hook names the binary by absolute path, not by PATH lookup.
		for _, want := range []string{"record -- ", " prompt"} {
			if !strings.Contains(out, want) {
				t.Errorf("the %s hook never calls %q:\n%s", shell, want, out)
			}
		}
	}
	if _, code := runBinary(t, t.TempDir(), nil, "init", "csh"); code != 2 {
		t.Error("a shell with no hook should be a usage error, not a silent success")
	}
}

func TestSettingsWritesThroughASymlink(t *testing.T) {
	// settings.json is very often a symlink into a dotfiles repo. Replacing the
	// link detaches the repo silently and leaves the old config in place.
	home := t.TempDir()
	claude := filepath.Join(home, ".claude")
	os.MkdirAll(claude, 0o755)
	real := filepath.Join(home, "dotfiles-settings.json")
	os.WriteFile(real, []byte(`{"model":"opus"}`), 0o644)
	link := filepath.Join(claude, "settings.json")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if out, code := runBinary(t, home, nil, "settings", "install"); code != 0 {
		t.Fatalf("settings install exited %d: %s", code, out)
	}
	fi, err := os.Lstat(link)
	if err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("the symlink was replaced by a regular file")
	}
	if !strings.Contains(statusLineCommand(t, real), "statusline") {
		t.Error("the wiring did not reach the file the link points at")
	}
}

func TestSettingsWiringIsReversible(t *testing.T) {
	home := t.TempDir()
	claude := filepath.Join(home, ".claude")
	os.MkdirAll(claude, 0o755)
	settings := filepath.Join(claude, "settings.json")
	os.WriteFile(settings, []byte(`{"model":"opus","statusLine":{"type":"command","command":"~/.claude/caveman.sh"}}`), 0o644)

	if out, code := runBinary(t, home, nil, "settings", "install"); code != 0 {
		t.Fatalf("settings install exited %d: %s", code, out)
	}
	cmdLine := statusLineCommand(t, settings)
	if !strings.Contains(cmdLine, "caveman") {
		t.Errorf("canary replaced what was already on the row: %q", cmdLine)
	}
	if !strings.Contains(cmdLine, "statusline") {
		t.Errorf("canary did not join the row: %q", cmdLine)
	}
	if _, err := os.Stat(settings + ".canary.bak"); err != nil {
		t.Error("no backup was taken of the person's own settings")
	}

	if out, _ := runBinary(t, home, nil, "settings", "install"); !strings.Contains(out, "already wired") {
		t.Errorf("a second install should be a no-op:\n%s", out)
	}

	runBinary(t, home, nil, "settings", "remove")
	if got := statusLineCommand(t, settings); got != "~/.claude/caveman.sh" {
		t.Errorf("uninstall did not leave the row as it found it: %q", got)
	}
}

func TestSettingsRefusesToRewriteJSONWithCommentsInIt(t *testing.T) {
	home := t.TempDir()
	claude := filepath.Join(home, ".claude")
	os.MkdirAll(claude, 0o755)
	settings := filepath.Join(claude, "settings.json")
	original := "{\n  // keep me\n  \"model\": \"opus\"\n}\n"
	os.WriteFile(settings, []byte(original), 0o644)

	out, code := runBinary(t, home, nil, "settings", "install")
	if code != 0 {
		t.Fatalf("exited %d: %s", code, out)
	}
	if !strings.Contains(out, "not plain JSON") {
		t.Errorf("no warning was given:\n%s", out)
	}
	if got, _ := os.ReadFile(settings); string(got) != original {
		t.Errorf("the comment was dropped:\n%s", got)
	}
}

func TestUnknownCommandIsAUsageError(t *testing.T) {
	out, code := runBinary(t, t.TempDir(), nil, "fly")
	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(out, "unknown command") {
		t.Errorf("no usage was printed:\n%s", out)
	}
}

func statusLineCommand(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var settings struct {
		StatusLine struct {
			Command string `json:"command"`
		} `json:"statusLine"`
	}
	if err := json.Unmarshal(raw, &settings); err != nil {
		t.Fatalf("settings.json is not valid JSON after canary touched it: %v\n%s", err, raw)
	}
	return settings.StatusLine.Command
}

// statuslineWith runs the status row against a Claude Code payload, with the
// phrase memory seeded to whatever the test needs to be true.
func statuslineWith(t *testing.T, home string, memory string, env ...string) string {
	t.Helper()
	os.MkdirAll(filepath.Join(home, ".canary"), 0o755)
	if memory != "" {
		if err := os.WriteFile(filepath.Join(home, ".canary", "phrase-state"), []byte(memory), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	transcript := filepath.Join(home, "transcript.jsonl")
	os.WriteFile(transcript, nil, 0o644)

	cmd := exec.Command(binary, "statusline")
	cmd.Env = append([]string{"HOME=" + home, "PATH=" + os.Getenv("PATH"), "COLUMNS=100"}, env...)
	cmd.Stdin = strings.NewReader(fmt.Sprintf(`{"cost":{"total_duration_ms":0},"transcript_path":%q}`, transcript))
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("statusline: %v", err)
	}
	return string(out)
}

func TestAPhraseHoldsAndThenTheBirdGoesQuiet(t *testing.T) {
	// The status row is redrawn several times a second. Without a hold a line
	// would flash once and be gone before it could be read; with one that never
	// expired, the bird would keep repeating itself at a state it left long ago.
	now := time.Now().Unix()
	const line = "the air is breathable. i'm bored."

	live := statuslineWith(t, t.TempDir(),
		fmt.Sprintf("state=fresh\nscore=0\nts=%d\nph_ts=%d\nph=%s\n", now, now, line))
	if !strings.Contains(live, line) {
		t.Errorf("a phrase inside its hold was dropped:\n%s", live)
	}

	expired := statuslineWith(t, t.TempDir(),
		fmt.Sprintf("state=fresh\nscore=0\nts=%d\nph_ts=%d\nph=%s\n", now, now-600, line))
	if strings.Contains(expired, line) {
		t.Errorf("a phrase outlived its hold:\n%s", expired)
	}
	if !strings.Contains(expired, "♪") {
		t.Errorf("the note should come back when the bird stops talking:\n%s", expired)
	}
}

func TestQuietSilencesThePhrasesNotTheBird(t *testing.T) {
	now := time.Now().Unix()
	out := statuslineWith(t, t.TempDir(),
		fmt.Sprintf("state=fresh\nscore=0\nts=%d\nph_ts=%d\nph=%s\n", now, now, "a line it would otherwise say"),
		"CANARY_QUIET=1")

	if strings.Contains(out, "⌐") {
		t.Errorf("CANARY_QUIET still drew a phrase:\n%s", out)
	}
	if !strings.Contains(out, "▐ O ▌>") {
		t.Errorf("CANARY_QUIET silenced the bird itself:\n%s", out)
	}
}

func TestPreviewDrawsEveryBand(t *testing.T) {
	// A contributor writing for `worn` needs to see `worn`, not the state their
	// own session happens to be in.
	home := t.TempDir()
	for band, art := range map[string]string{
		"fresh": "▐ O ▌>", "chirpy": "▐ ^ ▌>", "tired": "▐ - ▌>",
		"worn": "▐ ~ ▌>", "dead": "▐ x ▌v",
	} {
		out, code := runBinary(t, home, nil, "preview", "--state", band)
		if code != 0 {
			t.Fatalf("preview --state %s exited %d", band, code)
		}
		if !strings.Contains(out, art) {
			t.Errorf("preview --state %s drew the wrong bird:\n%s", band, out)
		}
	}
}

func TestLintChecksTheCorpusItShips(t *testing.T) {
	out, code := runBinary(t, t.TempDir(), nil, "lint")
	if code != 0 {
		t.Fatalf("the shipped corpus does not pass its own linter (exit %d):\n%s", code, out)
	}
	if !strings.Contains(out, "ok") {
		t.Errorf("lint said nothing:\n%s", out)
	}
}

func TestLintReportsABrokenCorpusAndFails(t *testing.T) {
	// A contributor runs this on a checkout, with no Go toolchain in sight.
	home := t.TempDir()
	corpus := filepath.Join(home, "phrases")
	os.MkdirAll(filepath.Join(corpus, "en", "states"), 0o755)
	os.WriteFile(filepath.Join(corpus, "en", "states", "fresh.txt"),
		[]byte("You should take a break!\n"), 0o644)

	out, code := runBinary(t, home, nil, "lint", corpus)
	if code != 1 {
		t.Fatalf("exit = %d, want 1:\n%s", code, out)
	}
	for _, want := range []string{"starts with a capital", `contains "should"`} {
		if !strings.Contains(out, want) {
			t.Errorf("lint did not report %q:\n%s", want, out)
		}
	}

	if _, code := runBinary(t, home, nil, "lint", filepath.Join(home, "nope")); code != 2 {
		t.Error("a missing directory should be a usage error")
	}
}

func TestTheNoteMovesOnlyDuringARealBreak(t *testing.T) {
	// The only pretty thing this tool does requires you to stop working to see
	// it, which is the same inversion the rare phrases use.
	now := time.Now().Unix()

	working := statuslineWith(t, t.TempDir(),
		fmt.Sprintf("state=fresh\nscore=0\nts=%d\nph_ts=0\nph=\n", now-5))
	if !strings.Contains(working, "♪") {
		t.Errorf("a working bird should show the static note:\n%s", working)
	}

	// Ten minutes since the last refresh: you were not at the keyboard.
	resting := statuslineWith(t, t.TempDir(),
		fmt.Sprintf("state=fresh\nscore=0\nts=%d\nph_ts=0\nph=\n", now-600))
	if strings.Contains(resting, "♪·") || strings.Contains(resting, "·♪") || strings.Contains(resting, "···") {
		return // an animation frame reached the row
	}
	if strings.HasSuffix(strings.TrimRight(resting, "\n"), "♪") {
		t.Errorf("the note did not move after a ten-minute gap:\n%s", resting)
	}
}

func TestTheBagAndSessionCountSurviveARefresh(t *testing.T) {
	// The shuffle state and the day's sessions are what make "without
	// replacement" and the seventh-session line possible at all; both live in
	// files because every refresh is a new process.
	home := t.TempDir()
	transcript := filepath.Join(home, "transcript.jsonl")
	os.WriteFile(transcript, []byte(`{"type":"user","message":{"content":"a first question about the thing"}}`+"\n"), 0o644)
	payload := fmt.Sprintf(`{"session_id":"abc-123","cwd":%q,"cost":{"total_duration_ms":600000},"transcript_path":%q}`, home, transcript)

	for i := 0; i < 3; i++ {
		cmd := exec.Command(binary, "statusline")
		cmd.Env = []string{"HOME=" + home, "PATH=" + os.Getenv("PATH"), "COLUMNS=100"}
		cmd.Stdin = strings.NewReader(payload)
		if _, err := cmd.Output(); err != nil {
			t.Fatalf("statusline: %v", err)
		}
	}

	sessions, err := os.ReadFile(filepath.Join(home, ".canary", "sessions"))
	if err != nil {
		t.Fatalf("no sessions file: %v", err)
	}
	if !strings.Contains(string(sessions), "abc-123") {
		t.Errorf("the session was not recorded:\n%s", sessions)
	}
	if strings.Count(string(sessions), "abc-123") != 1 {
		t.Errorf("three refreshes counted as more than one session:\n%s", sessions)
	}
}
