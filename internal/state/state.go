// Package state is the shell-prompt bird's memory.
//
// The shell version kept its counters in shell variables, so the bird was
// per-terminal: three windows open meant three birds, each ignorant of the
// others, while ~/.canary/canary-state — the file the Claude Code statusline
// reads — was overwritten by whichever of them ran a command last. The numbers
// on the status row jumped around because of it.
//
// A binary invoked once per prompt cannot hold anything in memory anyway, and
// the honest fix is the one that was always right: the counters belong to the
// person, not to the tty. One file, shared by every shell. The idle rule below
// is what makes that safe — a terminal you are not typing in accrues nothing.
//
// The file keeps the three keys the shell wrote, so an older statusline script
// can still read it, and adds the two the shell kept in memory.
package state

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"
)

// State is one session's accrued activity.
type State struct {
	PromptCount   int // commands run
	LenSum        int // running total of command lengths, in characters
	ActiveSeconds int // accrued time, idle gaps excluded
	LastActive    int // epoch seconds of the last recorded command
}

// AvgLen is the mean command length over the session. Cumulative, like minutes
// and count: a window of the last N lengths needed shell arcana (zsh's
// shwordsplit) for a term worth a couple of points out of a hundred.
func (s State) AvgLen() int {
	if s.PromptCount <= 0 {
		return 0
	}
	return s.LenSum / s.PromptCount
}

// Record returns the state after one command, without touching the receiver.
//
// Time only accrues across a gap shorter than idleThreshold. Lunch, a meeting
// or a night's sleep leave the terminal open, and counting that as work is what
// made the old linear bird meaningless by mid-afternoon.
func (s State) Record(cmd string, now, idleThreshold int) State {
	next := s
	if s.LastActive > 0 {
		if gap := now - s.LastActive; gap > 0 && gap <= idleThreshold {
			next.ActiveSeconds += gap
		}
	}
	next.LastActive = now
	next.PromptCount++
	// Characters, not bytes: bash's ${#var} counts characters in a UTF-8
	// locale, and canary requires a UTF-8 terminal to draw the bird at all.
	next.LenSum += utf8.RuneCountInString(cmd)
	return next
}

// Load reads the state file. A missing file means a fresh session rather than
// an error, and ok is false only when there is nothing usable to read — the
// caller decides whether that means "start at zero" (the prompt bird) or "say
// nothing at all" (the statusline, which must not invent a session).
//
// Symlinks are refused: this path is rewritten on every command, and following
// a link would let anything that can write to ~/.canary redirect that write.
func Load(path string) (State, bool) {
	fi, err := os.Lstat(path)
	if err != nil || fi.Mode()&os.ModeSymlink != 0 || !fi.Mode().IsRegular() {
		return State{}, false
	}
	f, err := os.Open(path)
	if err != nil {
		return State{}, false
	}
	defer f.Close()

	var s State
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, found := strings.Cut(sc.Text(), "=")
		if !found {
			continue
		}
		// A field is a number or it is nothing. The shell squeezed the digits
		// out of whatever it found (`tr -cd '0-9'`), which turned an injected
		// "\x1b[31m9" into 319: garbage in, a plausible-looking number out.
		// Rejecting the line is the honest reading, and still leaves nothing
		// on this path that could reach a prompt.
		n, ok := parseInt(v)
		if !ok {
			continue
		}
		switch k {
		case "prompt_count":
			s.PromptCount = n
		case "len_sum":
			s.LenSum = n
		case "active_seconds":
			s.ActiveSeconds = n
		case "last_active":
			s.LastActive = n
		}
	}
	// A file written by the shell bird has avg_prompt_len but no len_sum.
	// Reconstruct the sum so the average survives the upgrade instead of
	// resetting the moment the Go binary takes over.
	if s.LenSum == 0 && s.PromptCount > 0 {
		if avg := readInt(path, "avg_prompt_len"); avg > 0 {
			s.LenSum = avg * s.PromptCount
		}
	}
	return s, true
}

// Save writes the state atomically. Claude Code and impatient people both kill
// this process mid-run, and a half-written state file is a corrupt one.
//
// avg_prompt_len is written for readers that predate this package — the shell
// statusline computed the average itself and would see zero without it.
func Save(path string, s State) error {
	if fi, err := os.Lstat(path); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		return nil // refuse to follow a link, same as Load
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	var b strings.Builder
	b.WriteString("prompt_count=" + strconv.Itoa(s.PromptCount) + "\n")
	b.WriteString("avg_prompt_len=" + strconv.Itoa(s.AvgLen()) + "\n")
	b.WriteString("active_seconds=" + strconv.Itoa(s.ActiveSeconds) + "\n")
	b.WriteString("len_sum=" + strconv.Itoa(s.LenSum) + "\n")
	b.WriteString("last_active=" + strconv.Itoa(s.LastActive) + "\n")

	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp.*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no orphan temp files if anything below fails

	if _, err := tmp.WriteString(b.String()); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// readInt pulls one integer key out of the file, for the upgrade path above.
func readInt(path, key string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if k, v, found := strings.Cut(sc.Text(), "="); found && k == key {
			if n, ok := parseInt(v); ok {
				return n
			}
		}
	}
	return 0
}

// parseInt accepts a plain non-negative integer and nothing else. Surrounding
// whitespace is tolerated because a file edited by hand often has it.
func parseInt(s string) (int, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return n, true
}
