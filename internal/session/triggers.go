package session

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/thousandflowers/canary/internal/atomicfile"
)

// GitTTL is how long a working-tree answer is reused. The status row is redrawn
// several times a second; asking git that often would be the slowest thing
// canary does by a wide margin, and the answer does not change that fast.
const GitTTL = 60 * time.Second

// GitTimeout bounds the one call. A huge repository, a cold cache or a network
// filesystem must cost the bird a missing trigger, never a status row that
// failed to render.
const GitTimeout = 400 * time.Millisecond

// TriggerNames is every trigger with a detector behind it. The linter uses it
// to catch a phrases/triggers/<name>.txt that nothing will ever draw from —
// which is the failure VOICE.md warns about: adding a file there does nothing
// on its own.
var TriggerNames = []string{
	"same-file", "no-tests", "uncommitted", "compacted", "repeated-prompt", "interrupted",
}

// Triggers are the observations that have their own phrases. They are detected
// here, in code, and never by adding a file to phrases/triggers — which is why
// CONTRIBUTING says a new trigger is a code change and a new phrase is not.
//
// dirty comes from DirtyWorktree, which is the one signal the transcript cannot
// see. The order is stable so the pool a trigger picks is reproducible.
func (s Signals) Triggers(dirty bool) []string {
	var out []string
	if s.TopFileCount >= SameFilePasses {
		out = append(out, "same-file")
	}
	if s.Files >= MinFilesForNoTests && !s.HasTests {
		out = append(out, "no-tests")
	}
	if dirty {
		out = append(out, "uncommitted")
	}
	if s.Compacted {
		out = append(out, "compacted")
	}
	if s.RepeatedAsk {
		out = append(out, "repeated-prompt")
	}
	if s.Interrupted {
		out = append(out, "interrupted")
	}
	return out
}

// DirtyWorktree reports whether the directory Claude Code is working in has
// uncommitted changes.
//
// Answers are cached in a small file keyed by directory. A stale answer for up
// to a minute is the right trade here: the alternative is a git process per
// refresh, several times a second, for a joke about blind faith.
func DirtyWorktree(dir, cachePath string, now time.Time) bool {
	if dir == "" || !insideGitRepo(dir) {
		return false
	}
	if dirty, ok := readGitCache(cachePath, dir, now); ok {
		return dirty
	}

	ctx, cancel := context.WithTimeout(context.Background(), GitTimeout)
	defer cancel()
	// -uno: untracked files are not uncommitted work, they are usually build
	// output and editor litter, and counting them would make the trigger fire
	// for everyone forever.
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "status", "--porcelain", "-uno")
	out, err := cmd.Output()
	if err != nil {
		// No git, not a repo, or too slow. Say clean: a trigger that fires on
		// an error is a trigger that fires at random.
		return false
	}
	dirty := len(strings.TrimSpace(string(out))) > 0
	writeGitCache(cachePath, dir, dirty, now)
	return dirty
}

// insideGitRepo walks up looking for .git, so the common case — a directory
// that is not a repository at all — costs a few stat calls instead of a
// process.
func insideGitRepo(dir string) bool {
	for i := 0; i < 64; i++ {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return false
		}
		dir = parent
	}
	return false
}

func readGitCache(path, dir string, now time.Time) (bool, bool) {
	f, err := os.Open(path)
	if err != nil {
		return false, false
	}
	defer f.Close()

	var cachedDir string
	var ts int64
	dirty := false
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, found := strings.Cut(sc.Text(), "=")
		if !found {
			continue
		}
		switch k {
		case "dir":
			cachedDir = v
		case "ts":
			ts, _ = strconv.ParseInt(v, 10, 64)
		case "dirty":
			dirty = v == "1"
		}
	}
	if cachedDir != dir {
		return false, false // a different project; its answer is not this one's
	}
	if now.Unix()-ts > int64(GitTTL/time.Second) || now.Unix() < ts {
		return false, false
	}
	return dirty, true
}

func writeGitCache(path, dir string, dirty bool, now time.Time) {
	flag := "0"
	if dirty {
		flag = "1"
	}
	body := "dir=" + dir + "\nts=" + strconv.FormatInt(now.Unix(), 10) + "\ndirty=" + flag + "\n"
	// Best effort: a cache that could not be written costs one git call.
	_ = atomicfile.Write(path, []byte(body))
}
