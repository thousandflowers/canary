package session

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestTranscriptSignalsBehindTheTriggers(t *testing.T) {
	_, in := transcript(t,
		`{"type":"user","message":{"content":"the parser drops the last row, please look at internal/state"}}`,
		`{"type":"assistant","message":{"content":[{"type":"tool_use","input":{"file_path":"/repo/internal/state/state.go"}}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"tool_use","input":{"file_path":"/repo/internal/state/state.go"}}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"tool_use","input":{"file_path":"/repo/internal/state/state.go"}}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"tool_use","input":{"file_path":"/repo/internal/state/state.go"}}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"tool_use","input":{"file_path":"/repo/README.md"}}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"tool_use","input":{"file_path":"/repo/go.mod"}}]}}`,
		`{"type":"user","message":{"content":"the parser drops the last row, please look at internal/state"}}`,
		`{"type":"system","content":"[Request interrupted by user]"}`,
		`{"type":"summary","isCompactSummary":true}`,
	)

	got := FromClaudeCode(in)
	if got.TopFile != "/repo/internal/state/state.go" || got.TopFileCount != 4 {
		t.Errorf("top file = %q x%d, want state.go x4", got.TopFile, got.TopFileCount)
	}
	if got.Files != 3 {
		t.Errorf("distinct files = %d, want 3", got.Files)
	}
	if got.HasTests {
		t.Error("no test file was touched, but one was reported")
	}
	if !got.Compacted || !got.Interrupted || !got.RepeatedAsk {
		t.Errorf("missed a session signal: %+v", got)
	}
}

func TestATestFileIsRecognisedBySpelling(t *testing.T) {
	// The bird is not going to parse anyone's build system from a status line.
	for _, p := range []string{"internal/state/state_test.go", "tests/e2e.py", "src/foo.spec.ts", "spec/models/user.rb", "test_install.sh"} {
		if !isTestPath(p) {
			t.Errorf("%s should read as a test", p)
		}
	}
	for _, p := range []string{"internal/state/state.go", "README.md", "src/latest.ts"} {
		if isTestPath(p) {
			t.Errorf("%s is not a test", p)
		}
	}
}

func TestTriggersFireOnWhatWasSeen(t *testing.T) {
	sig := Signals{TopFileCount: SameFilePasses, Files: MinFilesForNoTests, Compacted: true, RepeatedAsk: true, Interrupted: true}
	got := sig.Triggers(true)
	for _, want := range []string{"same-file", "no-tests", "uncommitted", "compacted", "repeated-prompt", "interrupted"} {
		if !containsString(got, want) {
			t.Errorf("%q did not fire: %v", want, got)
		}
	}

	// And an ordinary session sets none of them off.
	quiet := Signals{TopFileCount: 2, Files: 2, HasTests: true}
	if got := quiet.Triggers(false); len(got) != 0 {
		t.Errorf("an ordinary session fired %v", got)
	}
}

func TestNoTestsNeedsMoreThanAFileOrTwo(t *testing.T) {
	// Two files and no test file is not a finding, it is a Tuesday.
	small := Signals{Files: MinFilesForNoTests - 1}
	if containsString(small.Triggers(false), "no-tests") {
		t.Error("no-tests fired on a session that had barely touched anything")
	}
	withTests := Signals{Files: 10, HasTests: true}
	if containsString(withTests.Triggers(false), "no-tests") {
		t.Error("no-tests fired on a session that wrote tests")
	}
}

func TestSameFileNeedsAFourthPass(t *testing.T) {
	// Read, edit, fix is ordinary work. The fourth pass is the one that starts
	// to look like an argument with a file.
	ordinary := Signals{TopFileCount: SameFilePasses - 1}
	if containsString(ordinary.Triggers(false), "same-file") {
		t.Error("same-file fired on ordinary work")
	}
}

func TestShortPromptsDoNotCountAsRepeats(t *testing.T) {
	// "yes", "go on" and "ok" repeat all day and mean nothing.
	_, in := transcript(t,
		`{"type":"user","message":{"content":"yes"}}`,
		`{"type":"user","message":{"content":"yes"}}`,
		`{"type":"user","message":{"content":"ok"}}`,
	)
	if FromClaudeCode(in).RepeatedAsk {
		t.Error("agreeing twice was read as being stuck")
	}
}

func TestCountTodayIsASetNotACounter(t *testing.T) {
	// The status row is redrawn several times a second and cancelled mid-run,
	// so anything incremented per refresh drifts within minutes.
	path := filepath.Join(t.TempDir(), "sessions")
	const today = 20692

	for i := 0; i < 5; i++ {
		if got := CountToday(path, "session-a", today); got != 1 {
			t.Fatalf("refresh %d counted %d sessions, want 1", i, got)
		}
	}
	if got := CountToday(path, "session-b", today); got != 2 {
		t.Errorf("a second session counted %d, want 2", got)
	}
	// Tomorrow starts over, without anything having to expire it.
	if got := CountToday(path, "session-c", today+1); got != 1 {
		t.Errorf("a new day counted %d, want 1", got)
	}
}

func TestShellModeHasNoSessionToCount(t *testing.T) {
	if got := CountToday(filepath.Join(t.TempDir(), "sessions"), "", 20692); got != 0 {
		t.Errorf("counted %d sessions without a session id", got)
	}
}

func TestDirtyWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("one\n"), 0o644)
	run("add", "a.txt")
	run("commit", "-qm", "first")

	cache := filepath.Join(t.TempDir(), "git-cache")
	if DirtyWorktree(dir, cache, time.Now()) {
		t.Error("a committed tree reported uncommitted work")
	}

	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("two\n"), 0o644)
	// A fresh cache path: the previous answer is still inside its TTL.
	if !DirtyWorktree(dir, filepath.Join(t.TempDir(), "git-cache"), time.Now()) {
		t.Error("an edited file did not read as uncommitted work")
	}
}

func TestAnswersAreCachedPerDirectory(t *testing.T) {
	// Asking git on every refresh would be the slowest thing canary does, by a
	// wide margin, for a joke about blind faith.
	cache := filepath.Join(t.TempDir(), "git-cache")
	now := time.Now()
	writeGitCache(cache, "/repo/one", true, now)

	if dirty, ok := readGitCache(cache, "/repo/one", now); !ok || !dirty {
		t.Errorf("the cached answer was not reused: dirty=%v ok=%v", dirty, ok)
	}
	if _, ok := readGitCache(cache, "/repo/two", now); ok {
		t.Error("another project's answer was reused")
	}
	if _, ok := readGitCache(cache, "/repo/one", now.Add(GitTTL+time.Second)); ok {
		t.Error("a stale answer was reused")
	}
}

func TestNoGitNoTrigger(t *testing.T) {
	// A directory that is not a repository costs a few stat calls, not a
	// process.
	dir := t.TempDir()
	if insideGitRepo(dir) {
		t.Skip("the temp dir is inside a repository; nothing to assert")
	}
	if DirtyWorktree(dir, filepath.Join(dir, "cache"), time.Now()) {
		t.Error("a directory with no git in it reported uncommitted work")
	}
	if DirtyWorktree("", "", time.Now()) {
		t.Error("an empty directory reported uncommitted work")
	}
}

func TestSessionIDAndDirComeOffThePayload(t *testing.T) {
	in := []byte(`{"session_id":"abc-123","cwd":"/fallback","workspace":{"current_dir":"/repo"},"cost":{"total_duration_ms":0},"transcript_path":""}`)
	got := FromClaudeCode(in)
	if got.SessionID != "abc-123" {
		t.Errorf("session id = %q", got.SessionID)
	}
	if got.Dir != "/repo" {
		t.Errorf("dir = %q, want the workspace's current dir", got.Dir)
	}

	noWorkspace := []byte(`{"cwd":"/fallback","cost":{"total_duration_ms":0},"transcript_path":""}`)
	if got := FromClaudeCode(noWorkspace).Dir; got != "/fallback" {
		t.Errorf("dir = %q, want the cwd when there is no workspace", got)
	}
}

func TestSessionIDsAreSanitisedOnTheWayBackIn(t *testing.T) {
	// This file is read back on every refresh; nothing that comes off disk gets
	// to carry an escape sequence anywhere near a status row.
	path := filepath.Join(t.TempDir(), "sessions")
	os.WriteFile(path, []byte("day=20692\nids=ok-1,\x1b[31mevil\n"), 0o644)

	// Reading is where the sanitising happens, so a file somebody else wrote
	// cannot smuggle an escape into anything downstream.
	if got := CountToday(path, "ok-1", 20692); got != 2 {
		t.Errorf("count = %d, want both ids", got)
	}
	// And the next real write puts the file back in a state canary would have
	// written itself.
	if got := CountToday(path, "ok-2", 20692); got != 3 {
		t.Errorf("count = %d after a new session, want 3", got)
	}
	raw, _ := os.ReadFile(path)
	if strings.ContainsRune(string(raw), 0x1b) {
		t.Errorf("an escape survived a rewrite: %q", raw)
	}
}

func TestTheDayIsPrunedToWhatItCanHold(t *testing.T) {
	// Nobody opens thirty-two Claude Code sessions in a day, and if they do the
	// bird has already said everything it has to say about it.
	path := filepath.Join(t.TempDir(), "sessions")
	const today = 20692
	for i := 0; i < MaxSessionsTracked+5; i++ {
		CountToday(path, "s-"+strconv.Itoa(i), today)
	}
	if got := CountToday(path, "s-last", today); got != MaxSessionsTracked {
		t.Errorf("count = %d, want the cap %d", got, MaxSessionsTracked)
	}
}

func TestCountingSurvivesADirectoryItCannotWrite(t *testing.T) {
	// The count is worth one ultra line. A status row is worth more.
	if os.Geteuid() == 0 {
		t.Skip("running as root; the mode would be ignored")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Skip(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o755) })

	if got := CountToday(filepath.Join(dir, "sessions"), "s-1", 20692); got != 1 {
		t.Errorf("count = %d, want the in-memory answer", got)
	}
}

func TestAGitCacheLineThatIsNotAFieldIsIgnored(t *testing.T) {
	path := filepath.Join(t.TempDir(), "git-cache")
	now := time.Now()
	os.WriteFile(path, []byte("a stray line\ndir=/repo\nts="+strconv.FormatInt(now.Unix(), 10)+"\ndirty=1\n"), 0o644)
	if dirty, ok := readGitCache(path, "/repo", now); !ok || !dirty {
		t.Errorf("a stray line broke the read: dirty=%v ok=%v", dirty, ok)
	}
}

func TestAnAnswerFromTheFutureIsNotReused(t *testing.T) {
	// A clock that jumped backwards would otherwise pin a stale answer in place
	// until it caught up.
	path := filepath.Join(t.TempDir(), "git-cache")
	now := time.Now()
	writeGitCache(path, "/repo", true, now.Add(time.Hour))
	if _, ok := readGitCache(path, "/repo", now); ok {
		t.Error("an answer stamped in the future was reused")
	}
}

func TestTheCachedAnswerIsWhatDirtyWorktreeReturns(t *testing.T) {
	// Proof that the git call is skipped: the directory is not a repository at
	// all, so a fresh answer would be false.
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0o755)
	cache := filepath.Join(t.TempDir(), "git-cache")
	now := time.Now()
	writeGitCache(cache, dir, true, now)

	if !DirtyWorktree(dir, cache, now) {
		t.Error("the cached answer was not used")
	}
}

func TestGitFailingIsNotUncommittedWork(t *testing.T) {
	// A trigger that fires on an error is a trigger that fires at random.
	dir := t.TempDir()
	// A .git that is a file, not a directory: the walk finds it, git does not
	// accept it.
	os.WriteFile(filepath.Join(dir, ".git"), []byte("not a repository"), 0o644)
	if DirtyWorktree(dir, filepath.Join(t.TempDir(), "cache"), time.Now()) {
		t.Error("a broken repository reported uncommitted work")
	}
}

func TestTheWalkUpStopsAtTheRoot(t *testing.T) {
	if insideGitRepo(string(filepath.Separator)) {
		t.Skip("the filesystem root is a git repository on this machine")
	}
}

func TestTheWalkUpGivesUpBeforeItGetsSilly(t *testing.T) {
	// A path deeper than the bound. Walking up forever from a pathological
	// directory is not worth a joke about uncommitted work.
	deep := t.TempDir()
	for i := 0; i < 70; i++ {
		deep = filepath.Join(deep, "d")
	}
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Skip(err)
	}
	if insideGitRepo(deep) {
		t.Skip("the temp directory is inside a repository")
	}
}
