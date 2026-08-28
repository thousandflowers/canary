package phrase

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
)

// HoldSeconds is how long a drawn line stays on screen. The statusline redraws
// several times a second; without a hold, a phrase would flash once and vanish
// before it could be read. After it expires the bird goes quiet again rather
// than drawing a new line, because the trigger is a state transition, not the
// passage of time.
const HoldSeconds = 60

// RecentKeep is how many recent lines are remembered, to stop the same one
// landing twice running.
const RecentKeep = 10

// Memory is what the bird knew at the previous refresh.
type Memory struct {
	Band   string
	Score  int
	TS     int64 // when that refresh happened
	PhTS   int64 // when the current phrase was drawn
	Phrase string
	// MineSession is the session an untranslated line was last drawn in. The
	// effect works once; as a standing mode it becomes noise or gets
	// auto-translated, so it is spent for the rest of that session.
	MineSession string
	// Known is false on the very first refresh, where there is no previous
	// score and so no trend — a different thing from a trend of zero.
	Known bool
}

// LoadMemory reads the phrase state file, tolerating anything in it. This runs
// on every statusline refresh and a corrupt line must cost silence, not a
// crash into Claude Code's status row.
func LoadMemory(path string) Memory {
	var m Memory
	fi, err := os.Lstat(path)
	if err != nil || fi.Mode()&os.ModeSymlink != 0 || !fi.Mode().IsRegular() {
		return m
	}
	f, err := os.Open(path)
	if err != nil {
		return m
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, found := strings.Cut(sc.Text(), "=")
		if !found {
			continue
		}
		switch k {
		case "state":
			m.Band = keepClass(v, unicode.IsLower)
		case "score":
			// Strict: a field with anything but digits in it is not a score.
			// Keeping only the digits would read an escape sequence as a number.
			if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n >= 0 {
				m.Score, m.Known = n, true
			}
		case "ts":
			m.TS = atoi64(v)
		case "ph_ts":
			m.PhTS = atoi64(v)
		case "ph":
			m.Phrase = keepClass(v, unicode.IsPrint)
		case "mine_session":
			m.MineSession = keepClass(v, func(r rune) bool {
				return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_'
			})
		}
	}
	return m
}

// SaveMemory writes the phrase state atomically.
func SaveMemory(path string, m Memory) error {
	var b strings.Builder
	b.WriteString("state=" + m.Band + "\n")
	b.WriteString("score=" + strconv.Itoa(m.Score) + "\n")
	b.WriteString("ts=" + strconv.FormatInt(m.TS, 10) + "\n")
	b.WriteString("ph_ts=" + strconv.FormatInt(m.PhTS, 10) + "\n")
	b.WriteString("ph=" + m.Phrase + "\n")
	b.WriteString("mine_session=" + m.MineSession + "\n")
	return writeAtomic(path, b.String())
}

// LoadRecent returns the lines the bird has used lately.
func LoadRecent(path string) []string {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out []string
	for _, line := range strings.Split(string(b), "\n") {
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// AppendRecent records one line, keeping the last RecentKeep.
func AppendRecent(path, line string) error {
	if line == "" {
		return nil
	}
	recent := append(LoadRecent(path), line)
	if len(recent) > RecentKeep {
		recent = recent[len(recent)-RecentKeep:]
	}
	return writeAtomic(path, strings.Join(recent, "\n")+"\n")
}

// writeAtomic writes via a temp file in the same directory. Claude Code cancels
// this process mid-run routinely; a half-written state file is a corrupt one,
// and the shell left `*.tmp.$$` litter behind every time it was killed between
// the write and the rename.
func writeAtomic(path, content string) error {
	if fi, err := os.Lstat(path); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		return nil // refuse to follow a link
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp.*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// keepClass drops every rune outside the class a field is allowed to hold.
// Parameter expansion did this in the shell; the point is the same one, that a
// file on disk cannot smuggle an escape sequence into the row.
func keepClass(s string, ok func(rune) bool) string {
	var b strings.Builder
	for _, r := range s {
		if ok(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func atoi64(s string) int64 {
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil || n < 0 {
		return 0
	}
	return n
}
