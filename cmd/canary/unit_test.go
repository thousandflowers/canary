package main

import (
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/thousandflowers/canary/internal/config"
	"github.com/thousandflowers/canary/internal/fatigue"
	"github.com/thousandflowers/canary/internal/phrase"
	"github.com/thousandflowers/canary/internal/session"
	"github.com/thousandflowers/canary/internal/state"
)

// These tests call the subcommands in this process. main_test.go runs the built
// binary instead, which proves the wiring; this file is where the paths that
// need a broken file, a read-only directory or a specific clock get exercised.

// capture swaps stdout and stderr for a pipe, runs fn, and returns what it
// printed alongside the code it returned.
func capture(t *testing.T, fn func() int) (string, int) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	outFile, errFile := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = w, w

	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()

	code := fn()

	w.Close()
	os.Stdout, os.Stderr = outFile, errFile
	return <-done, code
}

// isolate points every path canary writes at a throwaway home.
func isolate(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	t.Setenv("COLUMNS", "100")
	// Anything the ambient environment might have set, unset: these tests are
	// about the code, not about the machine they run on.
	for _, k := range []string{
		"CANARY_DISABLED", "CANARY_QUIET", "CANARY_ASCII", "CANARY_SHOW_SCORE",
		"CANARY_MIN_SCORE", "CANARY_RESET", "CANARY_PHRASE_DIR", "CANARY_NIGHT_MULT",
	} {
		t.Setenv(k, "")
	}
	return home
}

// withStdin points os.Stdin at a real file, which is what Claude Code's pipe
// looks like to the process.
func withStdin(t *testing.T, content string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stdin")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdin
	os.Stdin = f
	t.Cleanup(func() { os.Stdin = old; f.Close() })
}

func TestRunDispatchesEverySubcommand(t *testing.T) {
	isolate(t)
	noSleep(t) // `demo` is sixteen seconds of animation at its real pace
	cases := []struct {
		args []string
		want int
	}{
		{nil, 0}, // bare `canary` is status
		{[]string{"status"}, 0},
		{[]string{"score"}, 0},
		{[]string{"prompt"}, 0},
		{[]string{"record", "--", "git status"}, 0},
		{[]string{"reset"}, 0},
		{[]string{"chrono"}, 0},
		{[]string{"preview", "--state", "worn"}, 0},
		{[]string{"demo"}, 0},
		{[]string{"lint"}, 0},
		{[]string{"init", "zsh"}, 0},
		{[]string{"settings", "install"}, 0},
		{[]string{"statusline"}, 0},
		{[]string{"version"}, 0},
		{[]string{"--version"}, 0},
		{[]string{"-v"}, 0},
		{[]string{"help"}, 0},
		{[]string{"-h"}, 0},
		{[]string{"--help"}, 0},
		{[]string{"fly"}, 2},
	}
	for _, c := range cases {
		out, code := capture(t, func() int { return run(c.args) })
		if code != c.want {
			t.Errorf("canary %v: exit %d, want %d\n%s", c.args, code, c.want, out)
		}
	}
}

func TestUnknownCommandExplainsItself(t *testing.T) {
	isolate(t)
	out, code := capture(t, func() int { return run([]string{"fly"}) })
	if code != 2 || !strings.Contains(out, "unknown command") || !strings.Contains(out, "canary lint") {
		t.Errorf("exit %d, output:\n%s", code, out)
	}
}

func TestMainHandsRunsCodeToExit(t *testing.T) {
	isolate(t)
	oldArgs, oldExit := os.Args, exit
	t.Cleanup(func() { os.Args, exit = oldArgs, oldExit })

	got := -1
	exit = func(code int) { got = code }
	os.Args = []string{"canary", "fly"}
	capture(t, func() int { main(); return 0 })

	if got != 2 {
		t.Errorf("main exited %d, want the 2 run returned", got)
	}
}

func TestFailReportsAndReturnsOne(t *testing.T) {
	out, code := capture(t, func() int { return fail(os.ErrPermission) })
	if code != 1 || !strings.Contains(out, "permission denied") {
		t.Errorf("exit %d, output %q", code, out)
	}
}

func TestPromptRespectsTheThresholdAndTheOffSwitch(t *testing.T) {
	isolate(t)
	cfg := config.FromEnv()
	if err := state.Save(cfg.StateFile, state.State{PromptCount: 4, LenSum: 80, ActiveSeconds: 600, LastActive: int(time.Now().Unix())}); err != nil {
		t.Fatal(err)
	}

	drawn, _ := capture(t, func() int { return runPrompt(config.FromEnv()) })
	if !strings.Contains(drawn, "▌") {
		t.Errorf("the bird did not draw:\n%s", drawn)
	}

	t.Setenv("CANARY_MIN_SCORE", "99")
	quiet, code := capture(t, func() int { return runPrompt(config.FromEnv()) })
	if quiet != "" || code != 0 {
		t.Errorf("drew below the threshold: %q", quiet)
	}

	t.Setenv("CANARY_MIN_SCORE", "0")
	t.Setenv("CANARY_DISABLED", "1")
	off, _ := capture(t, func() int { return runPrompt(config.FromEnv()) })
	if off != "" {
		t.Errorf("drew while disabled: %q", off)
	}
	// record is a hook too, and obeys the same switch.
	if out, _ := capture(t, func() int { return runRecord(config.FromEnv(), []string{"--", "ls"}) }); out != "" {
		t.Errorf("record spoke while disabled: %q", out)
	}
}

func TestStatusAndScoreAlwaysAnswer(t *testing.T) {
	// Asking the bird directly is a different thing from it appearing uninvited.
	isolate(t)
	t.Setenv("CANARY_DISABLED", "1")

	status, code := capture(t, func() int { return runStatus(config.FromEnv()) })
	if code != 0 || !strings.Contains(status, "[fresh") {
		t.Errorf("status while disabled: %q", status)
	}
	score, _ := capture(t, func() int { return runScore(config.FromEnv()) })
	if strings.TrimSpace(score) != "0" {
		t.Errorf("score = %q, want 0", score)
	}
}

func TestRecordAccruesAndIgnoresABareEnter(t *testing.T) {
	isolate(t)
	cfg := config.FromEnv()

	capture(t, func() int { return runRecord(cfg, []string{"--", "git", "status"}) })
	if _, ok := state.Load(cfg.StateFile); !ok {
		t.Fatal("nothing was recorded")
	}
	capture(t, func() int { return runRecord(cfg, []string{"--", "   "}) })
	got, _ := state.Load(cfg.StateFile)
	if got.PromptCount != 1 {
		t.Errorf("a bare Enter was counted: %d commands", got.PromptCount)
	}
	// No `--`, and no arguments at all: both are ordinary calls from a hook.
	capture(t, func() int { return runRecord(cfg, []string{"vim"}) })
	capture(t, func() int { return runRecord(cfg, nil) })
	if got, _ := state.Load(cfg.StateFile); got.PromptCount != 2 {
		t.Errorf("commands = %d, want 2", got.PromptCount)
	}
}

func TestResetStartsOver(t *testing.T) {
	isolate(t)
	cfg := config.FromEnv()
	state.Save(cfg.StateFile, state.State{PromptCount: 9, LenSum: 900, ActiveSeconds: 9000})

	out, code := capture(t, func() int { return runReset(cfg) })
	if code != 0 || !strings.Contains(out, "canary: reset") {
		t.Errorf("exit %d, output %q", code, out)
	}
	if got, _ := state.Load(cfg.StateFile); got.PromptCount != 0 || got.ActiveSeconds != 0 {
		t.Errorf("state survived the reset: %+v", got)
	}
}

func TestResetReportsAWriteItCouldNotMake(t *testing.T) {
	// The explicit commands complain; the hooks stay quiet. This is one of the
	// former.
	isolate(t)
	blocked := filepath.Join(t.TempDir(), "a-file")
	os.WriteFile(blocked, nil, 0o644)
	t.Setenv("CANARY_STATE_FILE", filepath.Join(blocked, "nested", "canary-state"))

	out, code := capture(t, func() int { return runReset(config.FromEnv()) })
	if code != 1 || !strings.Contains(out, "canary:") {
		t.Errorf("exit %d, output %q", code, out)
	}
}

func TestCanaryResetEnvStillWorks(t *testing.T) {
	// `export CANARY_RESET=1` is muscle memory for anyone who has been running
	// canary since it was a shell script.
	isolate(t)
	cfg := config.FromEnv()
	state.Save(cfg.StateFile, state.State{PromptCount: 40, LenSum: 4000, ActiveSeconds: 36000})

	t.Setenv("CANARY_RESET", "1")
	out, _ := capture(t, func() int { return runStatus(config.FromEnv()) })
	if !strings.Contains(out, "[fresh 0]") {
		t.Errorf("the session was not reset:\n%s", out)
	}
	if got, _ := state.Load(cfg.StateFile); got.PromptCount != 0 {
		t.Errorf("the file kept the old counters: %+v", got)
	}
}

func TestPromptScoreIgnoresDebt(t *testing.T) {
	// The prompt bird scores the session in front of you; debt belongs to the
	// status row, which has room to explain itself.
	cfg := config.Config{NightMult: 100}
	st := state.State{PromptCount: 10, LenSum: 200, ActiveSeconds: 3600}
	noon := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	want := fatigue.ShellRaw(60, 10, 20)
	if got := promptScore(cfg, st, noon); got != want {
		t.Errorf("promptScore = %d, want %d", got, want)
	}
}

func TestPreviewFlagsAndTheirErrors(t *testing.T) {
	isolate(t)
	cfg := config.FromEnv()

	for _, band := range []string{"fresh", "chirpy", "tired", "worn", "dead"} {
		out, code := capture(t, func() int { return runPreview(cfg, []string{"--state", band, "--note", "falling"}) })
		if code != 0 || !strings.Contains(out, band) {
			t.Errorf("preview %s: exit %d, output %q", band, code, out)
		}
	}
	if out, code := capture(t, func() int { return runPreview(cfg, []string{"--phrase", "a candidate line"}) }); code != 0 || !strings.Contains(out, "a candidate line") {
		t.Errorf("explicit phrase: exit %d, output %q", code, out)
	}

	bad := [][]string{
		{"--state"},              // a flag with nothing after it
		{"--state", "exhausted"}, // a band that does not exist
		{"--colour", "green"},    // a flag that does not exist
	}
	for _, args := range bad {
		if _, code := capture(t, func() int { return runPreview(cfg, args) }); code != 2 {
			t.Errorf("preview %v should be a usage error", args)
		}
	}
}

func TestParseBand(t *testing.T) {
	if b, ok := parseBand("worn"); !ok || b != fatigue.Worn {
		t.Errorf("worn parsed as %q ok=%v", b, ok)
	}
	if _, ok := parseBand("exhausted"); ok {
		t.Error("a band that does not exist was accepted")
	}
}

func TestInitPrintsAHookThatNamesThisBinary(t *testing.T) {
	for _, shell := range []string{"zsh", "bash", "fish"} {
		out, code := capture(t, func() int { return runInit([]string{shell}) })
		if code != 0 {
			t.Fatalf("init %s: exit %d", shell, code)
		}
		if !strings.Contains(out, binaryPath()) {
			t.Errorf("the %s hook does not name this binary:\n%s", shell, out)
		}
		if strings.Contains(out, "{{canary}}") {
			t.Errorf("the %s hook still has its placeholder in it:\n%s", shell, out)
		}
	}
	if _, code := capture(t, func() int { return runInit([]string{"csh"}) }); code != 2 {
		t.Error("a shell with no hook should be a usage error")
	}
	if _, code := capture(t, func() int { return runInit(nil) }); code != 2 {
		t.Error("init with no shell should be a usage error")
	}
}

func TestBinaryPathResolves(t *testing.T) {
	// Absolute, because Claude Code's PATH is not the shell's.
	if got := binaryPath(); !filepath.IsAbs(got) {
		t.Errorf("binaryPath = %q, want an absolute path", got)
	}
}

func TestLintTargets(t *testing.T) {
	home := isolate(t)
	cfg := config.FromEnv()

	if out, code := capture(t, func() int { return runLint(cfg, nil) }); code != 0 || !strings.Contains(out, "ok") {
		t.Errorf("the shipped corpus failed its own linter: exit %d\n%s", code, out)
	}

	bad := filepath.Join(home, "corpus")
	os.MkdirAll(filepath.Join(bad, "en", "states"), 0o755)
	os.WriteFile(filepath.Join(bad, "en", "states", "fresh.txt"), []byte("You should stop!\n"), 0o644)
	out, code := capture(t, func() int { return runLint(cfg, []string{bad}) })
	if code != 1 || !strings.Contains(out, "starts with a capital") {
		t.Errorf("exit %d, output:\n%s", code, out)
	}

	if _, code := capture(t, func() int { return runLint(cfg, []string{filepath.Join(home, "nope")}) }); code != 2 {
		t.Error("a missing directory should be a usage error")
	}
	if _, code := capture(t, func() int { return runLint(cfg, []string{bad, bad}) }); code != 2 {
		t.Error("two directories should be a usage error")
	}
}

func TestSettingsUsage(t *testing.T) {
	isolate(t)
	cfg := config.FromEnv()
	for _, args := range [][]string{nil, {"install", "remove"}, {"rewire"}} {
		if _, code := capture(t, func() int { return runSettings(cfg, args) }); code != 2 {
			t.Errorf("settings %v should be a usage error", args)
		}
	}
}

func TestSettingsPathFollowsClaudeConfigDir(t *testing.T) {
	home := isolate(t)
	// The default, when nothing points elsewhere.
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	if got := settingsPath(config.FromEnv()); got != filepath.Join(home, ".claude", "settings.json") {
		t.Errorf("default settings path = %q", got)
	}
	t.Setenv("CLAUDE_CONFIG_DIR", "/somewhere/else")
	if got := settingsPath(config.FromEnv()); got != "/somewhere/else/settings.json" {
		t.Errorf("override ignored: %q", got)
	}
}

func TestSettingsSkipsAMachineWithNoClaudeCode(t *testing.T) {
	// Plenty of people only ever want the shell bird. That is not a failure.
	isolate(t)
	out, code := capture(t, func() int { return runSettings(config.FromEnv(), []string{"install"}) })
	if code != 0 || !strings.Contains(out, "skipping the status line") {
		t.Errorf("exit %d, output %q", code, out)
	}
}

func TestSettingsInstallOnAFreshConfig(t *testing.T) {
	home := isolate(t)
	claude := filepath.Join(home, ".claude")
	os.MkdirAll(claude, 0o755)

	out, code := capture(t, func() int { return runSettings(config.FromEnv(), []string{"install"}) })
	if code != 0 || !strings.Contains(out, "wired into") {
		t.Errorf("exit %d, output %q", code, out)
	}
	settings, ok := readSettings(filepath.Join(claude, "settings.json"))
	if !ok || !strings.Contains(currentCommand(settings), "statusline") {
		t.Errorf("nothing was wired: %+v", settings)
	}

	// A second install is a no-op rather than a second segment.
	again, _ := capture(t, func() int { return runSettings(config.FromEnv(), []string{"install"}) })
	if !strings.Contains(again, "already wired") {
		t.Errorf("a second install said %q", again)
	}
}

func TestSettingsRemoveHandlesEveryShapeItFinds(t *testing.T) {
	home := isolate(t)
	claude := filepath.Join(home, ".claude")
	os.MkdirAll(claude, 0o755)
	path := filepath.Join(claude, "settings.json")
	cfg := config.FromEnv()

	// Nothing there at all.
	if _, code := capture(t, func() int { return runSettings(cfg, []string{"remove"}) }); code != 0 {
		t.Error("removing from a missing file should be a quiet no-op")
	}
	// A settings.json with no status line in it.
	os.WriteFile(path, []byte(`{"model":"opus"}`), 0o644)
	if _, code := capture(t, func() int { return runSettings(cfg, []string{"remove"}) }); code != 0 {
		t.Error("removing when there is no status line should be a quiet no-op")
	}
	// Canary alone: the whole key goes.
	capture(t, func() int { return runSettings(cfg, []string{"install"}) })
	capture(t, func() int { return runSettings(cfg, []string{"remove"}) })
	settings, _ := readSettings(path)
	if _, ok := settings["statusLine"]; ok {
		t.Errorf("the empty statusLine key was left behind: %+v", settings)
	}
	// Sharing the row: only canary's segment goes.
	os.WriteFile(path, []byte(`{"statusLine":{"type":"command","command":"bash /opt/caveman.sh"}}`), 0o644)
	capture(t, func() int { return runSettings(cfg, []string{"install"}) })
	capture(t, func() int { return runSettings(cfg, []string{"remove"}) })
	settings, _ = readSettings(path)
	if got := currentCommand(settings); got != "bash /opt/caveman.sh" {
		t.Errorf("the row was not left as it was found: %q", got)
	}
	// JSONC is left alone in both directions.
	const jsonc = "{\n  // keep me\n  \"model\": \"opus\"\n}\n"
	os.WriteFile(path, []byte(jsonc), 0o644)
	capture(t, func() int { return runSettings(cfg, []string{"remove"}) })
	if got, _ := os.ReadFile(path); string(got) != jsonc {
		t.Errorf("a commented settings.json was rewritten:\n%s", got)
	}
}

func TestSettingsReportsAFileItCannotWrite(t *testing.T) {
	home := isolate(t)
	claude := filepath.Join(home, ".claude")
	os.MkdirAll(claude, 0o755)
	path := filepath.Join(claude, "settings.json")
	os.WriteFile(path, []byte(`{"statusLine":{"type":"command","command":"x canary statusline"}}`), 0o644)

	// A read-only directory: the temp file the atomic write needs cannot be
	// created. Root ignores the mode, so skip there rather than assert a lie.
	if os.Geteuid() == 0 {
		t.Skip("running as root; the mode would be ignored")
	}
	if err := os.Chmod(claude, 0o555); err != nil {
		t.Skip(err)
	}
	t.Cleanup(func() { os.Chmod(claude, 0o755) })

	if _, code := capture(t, func() int { return runSettings(config.FromEnv(), []string{"remove"}) }); code != 1 {
		t.Error("a failed write should be reported, not swallowed")
	}
	os.WriteFile(path, nil, 0o644) // no-op if it fails; the dir is read-only
	if _, code := capture(t, func() int { return runSettings(config.FromEnv(), []string{"install"}) }); code != 1 {
		t.Error("a failed install write should be reported")
	}
}

func TestReadSettingsToleratesWhatItFinds(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "absent.json")
	if got, ok := readSettings(missing); !ok || len(got) != 0 {
		t.Errorf("a missing file should read as an empty config: %+v ok=%v", got, ok)
	}
	empty := filepath.Join(dir, "empty.json")
	os.WriteFile(empty, []byte("   \n"), 0o644)
	if got, ok := readSettings(empty); !ok || len(got) != 0 {
		t.Errorf("an empty file should read as an empty config: %+v ok=%v", got, ok)
	}
	broken := filepath.Join(dir, "broken.json")
	os.WriteFile(broken, []byte("{not json"), 0o644)
	if _, ok := readSettings(broken); ok {
		t.Error("unparseable JSON was accepted for rewriting")
	}
	// A directory in place of the file: unreadable, and not ours to rewrite.
	if _, ok := readSettings(dir); ok {
		t.Error("a directory was accepted as settings")
	}
}

func TestCurrentCommandSurvivesAnyShape(t *testing.T) {
	// This file belongs to the person, not to canary. Whatever is in it, the
	// answer is a string.
	cases := []map[string]any{
		{},
		{"statusLine": "a bare string"},
		{"statusLine": map[string]any{}},
		{"statusLine": map[string]any{"command": 42}},
	}
	for _, settings := range cases {
		if got := currentCommand(settings); got != "" {
			t.Errorf("%v: got %q, want empty", settings, got)
		}
	}
	if got := currentCommand(map[string]any{"statusLine": map[string]any{"command": "x"}}); got != "x" {
		t.Errorf("got %q, want x", got)
	}
}

func TestShellQuoteSurvivesAnAwkwardPath(t *testing.T) {
	// Paths with spaces are ordinary on macOS, and one unquoted space turns the
	// status line into a command not found.
	got := shellQuote(`/Users/x/Progetti dev/ca"na$ry` + "`x`")
	for _, raw := range []string{`\"`, `\$`, "\\`"} {
		if !strings.Contains(got, raw) {
			t.Errorf("%s was not escaped: %s", raw, got)
		}
	}
	if !strings.HasPrefix(got, `"`) || !strings.HasSuffix(got, `"`) {
		t.Errorf("not quoted: %s", got)
	}
}

func TestStatuslineSaysNothingWhenItHasNothingTrueToSay(t *testing.T) {
	isolate(t)
	withStdin(t, "")
	out, code := capture(t, func() int { return runStatusline(config.FromEnv()) })
	if out != "" || code != 0 {
		t.Errorf("invented a session: %q (exit %d)", out, code)
	}

	t.Setenv("CANARY_DISABLED", "1")
	if out, _ := capture(t, func() int { return runStatusline(config.FromEnv()) }); out != "" {
		t.Errorf("spoke while disabled: %q", out)
	}
}

func TestStatuslineReadsBothWorlds(t *testing.T) {
	home := isolate(t)
	cfg := config.FromEnv()

	// Shell mode: the state file the prompt bird keeps.
	state.Save(cfg.StateFile, state.State{PromptCount: 6, LenSum: 120, ActiveSeconds: 1800})
	withStdin(t, "")
	shell, _ := capture(t, func() int { return runStatusline(config.FromEnv()) })
	if !strings.Contains(shell, "6p") {
		t.Errorf("shell mode:\n%s", shell)
	}

	// Claude Code mode: its JSON, and the transcript it points at.
	transcript := filepath.Join(home, "transcript.jsonl")
	os.WriteFile(transcript, []byte(`{"type":"user","message":{"content":"why is this failing again"}}`+"\n"), 0o644)
	withStdin(t, `{"session_id":"s1","cwd":"`+home+`","cost":{"total_duration_ms":3600000},"transcript_path":"`+transcript+`"}`)
	t.Setenv("CANARY_SHOW_SCORE", "1")
	cc, _ := capture(t, func() int { return runStatusline(config.FromEnv()) })
	if !strings.Contains(cc, "60m") || !strings.Contains(cc, "1t") {
		t.Errorf("Claude Code mode:\n%s", cc)
	}
}

func TestStatuslineHonoursTheQuietThreshold(t *testing.T) {
	isolate(t)
	state.Save(config.FromEnv().StateFile, state.State{PromptCount: 2, ActiveSeconds: 60})
	t.Setenv("CANARY_MIN_SCORE", "71")
	withStdin(t, "")
	if out, _ := capture(t, func() int { return runStatusline(config.FromEnv()) }); out != "" {
		t.Errorf("drew below the threshold:\n%s", out)
	}
}

func TestQuietKeepsTheBirdAndDropsTheWords(t *testing.T) {
	isolate(t)
	cfg := config.FromEnv()
	state.Save(cfg.StateFile, state.State{PromptCount: 2, ActiveSeconds: 60})
	t.Setenv("CANARY_QUIET", "1")
	withStdin(t, "")

	out, _ := capture(t, func() int { return runStatusline(config.FromEnv()) })
	if strings.Contains(out, "⌐") {
		t.Errorf("CANARY_QUIET drew a phrase:\n%s", out)
	}
	if !strings.Contains(out, "▌") {
		t.Errorf("CANARY_QUIET silenced the bird itself:\n%s", out)
	}
}

func TestReadStdinRefusesATerminal(t *testing.T) {
	// Reading from a tty would block the status row forever, waiting for a
	// person to type into a script they cannot see.
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Skip(err)
	}
	defer devNull.Close()
	old := os.Stdin
	os.Stdin = devNull
	t.Cleanup(func() { os.Stdin = old })

	if got := readStdin(); got != nil {
		t.Errorf("read %q from a character device", got)
	}
}

func TestReadStdinSurvivesAClosedDescriptor(t *testing.T) {
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Skip(err)
	}
	f.Close()
	old := os.Stdin
	os.Stdin = f
	t.Cleanup(func() { os.Stdin = old })

	if got := readStdin(); got != nil {
		t.Errorf("read %q from a closed descriptor", got)
	}
}

func TestSlotsForOffersOnlyWhatTheBirdKnows(t *testing.T) {
	noon := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	empty := slotsFor(session.Signals{}, noon)
	if _, ok := empty["file"]; ok {
		t.Error("a file slot was offered with no file behind it")
	}
	if empty["time"] != "this morning" {
		t.Errorf("time = %q", empty["time"])
	}

	full := slotsFor(session.Signals{TopFile: "/repo/internal/state/state.go", TopFileCount: 6, Dir: "/repo"}, noon)
	if full["file"] != "state.go" || full["repo"] != "repo" || full["n"] != "6" {
		t.Errorf("slots = %+v", full)
	}
}

func TestCorpusPrefersADirectoryWhenOneIsGiven(t *testing.T) {
	home := isolate(t)
	dir := filepath.Join(home, "corpus")
	os.MkdirAll(filepath.Join(dir, "en", "states"), 0o755)
	os.WriteFile(filepath.Join(dir, "en", "states", "worn.txt"), []byte("a line only this corpus has.\n"), 0o644)
	t.Setenv("CANARY_PHRASE_DIR", dir)

	got := corpus(config.FromEnv()).Lines("en/states/worn.txt")
	if len(got) != 1 || got[0] != "a line only this corpus has." {
		t.Errorf("the directory did not override the embedded corpus: %q", got)
	}
}

func TestGlobalRandStaysInRange(t *testing.T) {
	for i := 0; i < 200; i++ {
		if got := (globalRand{}).IntN(7); got < 0 || got >= 7 {
			t.Fatalf("IntN(7) = %d", got)
		}
	}
}

func TestBinaryPathFallsBackToPath(t *testing.T) {
	// The one branch here that running the binary cannot reach: if the process
	// cannot say where it is, PATH is all that is left.
	old := executable
	t.Cleanup(func() { executable = old })

	executable = func() (string, error) { return "", os.ErrNotExist }
	if got := binaryPath(); got != "canary" {
		t.Errorf("binaryPath = %q, want the PATH fallback", got)
	}

	// And a path that cannot be resolved is used as it stands, rather than
	// discarded: an unresolvable path still runs.
	executable = func() (string, error) { return "/nonexistent/canary", nil }
	if got := binaryPath(); got != "/nonexistent/canary" {
		t.Errorf("binaryPath = %q", got)
	}
}

func TestBinaryPathKeepsTheSymlinkHomebrewInstalls(t *testing.T) {
	// Homebrew's bin/canary points into a version-pinned Cellar directory that
	// the next `brew cleanup` deletes. The status line command is written into
	// settings.json once and never rewritten, so resolving the link there
	// leaves behind a path that stops existing after an upgrade.
	dir := t.TempDir()
	cellar := filepath.Join(dir, "Cellar", "canary", "1.1.0", "bin")
	if err := os.MkdirAll(cellar, 0o755); err != nil {
		t.Fatal(err)
	}
	real := filepath.Join(cellar, "canary")
	if err := os.WriteFile(real, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "canary")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}

	old := executable
	t.Cleanup(func() { executable = old })
	executable = func() (string, error) { return link, nil }

	if got := binaryPath(); got != link {
		t.Errorf("binaryPath = %q, want the symlink itself %q", got, link)
	}
}

func TestSettingsInstallRefusesToRewriteJSONWithCommentsInIt(t *testing.T) {
	home := isolate(t)
	claude := filepath.Join(home, ".claude")
	os.MkdirAll(claude, 0o755)
	path := filepath.Join(claude, "settings.json")
	const jsonc = "{\n  // keep me\n  \"model\": \"opus\"\n}\n"
	os.WriteFile(path, []byte(jsonc), 0o644)

	out, code := capture(t, func() int { return runSettings(config.FromEnv(), []string{"install"}) })
	if code != 0 || !strings.Contains(out, "not plain JSON") {
		t.Errorf("exit %d, output %q", code, out)
	}
	if !strings.Contains(out, "statusline") {
		t.Errorf("no manual instruction was given:\n%s", out)
	}
	if got, _ := os.ReadFile(path); string(got) != jsonc {
		t.Errorf("the comment was dropped:\n%s", got)
	}
}

func TestWriteSettingsReportsWhatItCannotDo(t *testing.T) {
	// Both halves of the atomic write, tested where they can actually be made
	// to fail: through runSettings the file would be unreadable first, and the
	// JSON guard would stop the whole thing earlier.
	dir := t.TempDir()

	// A directory where the temp file goes.
	blocked := filepath.Join(dir, "blocked.json")
	os.MkdirAll(blocked+".canary.tmp", 0o755)
	if err := writeSettings(blocked, map[string]any{"model": "opus"}); err == nil {
		t.Error("a temp file that cannot be written should be reported")
	}

	// A directory where the settings file goes: the write lands, the rename
	// cannot.
	occupied := filepath.Join(dir, "occupied.json")
	os.MkdirAll(filepath.Join(occupied, "in the way"), 0o755)
	if err := writeSettings(occupied, map[string]any{"model": "opus"}); err == nil {
		t.Error("a rename that cannot happen should be reported")
	}
	if _, err := os.Stat(occupied + ".canary.tmp"); err == nil {
		t.Error("the temp file survived a failed rename")
	}
}

func TestStatuslineCountsTonightTowardsTheStreak(t *testing.T) {
	// Today extends the streak while it is happening, not the next morning.
	isolate(t)
	cfg := config.FromEnv()
	state.Save(cfg.StateFile, state.State{PromptCount: 400, LenSum: 40000, ActiveSeconds: 86400})
	// Two nights already past the limit, so the count is printed rather than
	// only carried.
	today := int(time.Now().Unix() / 86400)
	os.MkdirAll(filepath.Dir(cfg.HistoryFile), 0o755)
	os.WriteFile(cfg.HistoryFile, []byte(
		strconv.Itoa(today-1)+" 99\n"+strconv.Itoa(today-2)+" 99\n"), 0o644)
	withStdin(t, "")

	out, _ := capture(t, func() int { return runStatusline(config.FromEnv()) })
	if !strings.Contains(out, "nights past your limit") {
		t.Errorf("a third night was not counted:\n%s", out)
	}
}

func TestReadStdinSurvivesAThingItCannotRead(t *testing.T) {
	// A directory: not a character device, so it is read — and reading it
	// fails. The bird goes quiet rather than crashing into the status row.
	dir, err := os.Open(t.TempDir())
	if err != nil {
		t.Skip(err)
	}
	defer dir.Close()
	old := os.Stdin
	os.Stdin = dir
	t.Cleanup(func() { os.Stdin = old })

	if got := readStdin(); got != nil {
		t.Errorf("read %q out of a directory", got)
	}
}

func TestAPhraseHoldsForAMinuteThenTheBirdGoesQuiet(t *testing.T) {
	isolate(t)
	cfg := config.FromEnv()
	state.Save(cfg.StateFile, state.State{PromptCount: 2, ActiveSeconds: 120})
	withStdin(t, "")
	now := time.Now().Unix()
	const line = "the air is breathable. i'm bored."

	// Same band, inside the hold: the line stays on screen across the several
	// refreshes a second Claude Code asks for.
	phrase.SaveMemory(cfg.PhraseState, phrase.Memory{
		Band: "fresh", Score: 1, TS: now, PhTS: now, Phrase: line,
	})
	held, _ := capture(t, func() int { return runStatusline(config.FromEnv()) })
	if !strings.Contains(held, line) {
		t.Errorf("a phrase inside its hold was dropped:\n%s", held)
	}

	// Past it: quiet again, rather than something new to say.
	phrase.SaveMemory(cfg.PhraseState, phrase.Memory{
		Band: "fresh", Score: 1, TS: now, PhTS: now - phrase.HoldSeconds - 1, Phrase: line,
	})
	expired, _ := capture(t, func() int { return runStatusline(config.FromEnv()) })
	if strings.Contains(expired, line) {
		t.Errorf("a phrase outlived its hold:\n%s", expired)
	}
}

// forcedRare rolls past the silence gate and takes the rare tier, whatever else
// it is offered.
type forcedRare struct{}

func (forcedRare) IntN(n int) int {
	switch n {
	case 100:
		return 99
	case phrase.UltraOdds:
		return 1 // not ultra
	default:
		return 0 // rare, and then the first line of the shuffle
	}
}

func TestAnUntranslatedLineIsSpentForTheSession(t *testing.T) {
	// The effect works once. As a standing mode it becomes noise, or it gets
	// pasted into a translator, which is the same thing.
	home := isolate(t)
	corpusDir := filepath.Join(home, "corpus")
	os.MkdirAll(filepath.Join(corpusDir, "en", "states"), 0o755)
	os.MkdirAll(filepath.Join(corpusDir, "mine"), 0o755)
	os.WriteFile(filepath.Join(corpusDir, "en", "states", "fresh.txt"), []byte("an ordinary line.\n"), 0o644)
	os.WriteFile(filepath.Join(corpusDir, "mine", "untranslated.txt"), []byte("sisu.\n"), 0o644)
	t.Setenv("CANARY_PHRASE_DIR", corpusDir)

	oldDice := dice
	dice = forcedRare{}
	t.Cleanup(func() { dice = oldDice })

	cfg := config.FromEnv()
	state.Save(cfg.StateFile, state.State{PromptCount: 1, ActiveSeconds: 30})
	withStdin(t, `{"session_id":"s-1","cost":{"total_duration_ms":30000},"transcript_path":""}`)

	out, _ := capture(t, func() int { return runStatusline(config.FromEnv()) })
	if !strings.Contains(out, "sisu.") {
		t.Fatalf("the rare tier did not reach mine/:\n%s", out)
	}
	if got := phrase.LoadMemory(cfg.PhraseState).MineSession; got != "s-1" {
		t.Errorf("the untranslated line was not spent: mine_session = %q", got)
	}
	if got := phrase.LoadRecent(cfg.RecentFile); len(got) != 1 || got[0] != "sisu." {
		t.Errorf("the line was not remembered as recent: %q", got)
	}
}
