// Package phrase decides whether the bird says anything, and if so what.
//
// The rules are VOICE.md's, not this file's, and two of them are load-bearing:
// the rarest lines are gated on RECOVERING, never on hours logged — an
// encounter system that paid you for a long session would invert a tool about
// fatigue — and there is no counter, no dex, no "12/40 seen".
//
// The corpus ships inside the binary. The shell looked for ~/.canary/phrases
// and then for a phrases/ directory beside the script, and shipped mute
// whenever the packaging forgot to install it; that failure mode is gone. A
// directory still overrides the embedded copy, for contributors iterating on a
// line without rebuilding.
package phrase

import (
	"io/fs"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// DefaultLang is the only language shipped so far. Translations live beside it
// (phrases/it, phrases/fr) and `mine/` sits outside all of them on purpose:
// its lines are never translated, which is the whole point of them.
const DefaultLang = "en"

// EphemeralYears is how many years of topical lines stay readable: this one and
// the last. A 2026 line stops being drawn in 2028 without anybody deciding
// anything, and stays in the repo as archive. Maintenance solved structurally.
const EphemeralYears = 2

// Corpus is a phrase tree. Paths are given relative to its root, and the
// language directory is part of them — `mine/` has to be reachable from the
// same reader, and it is deliberately not inside any language.
type Corpus struct {
	fsys fs.FS
	root string
	lang string
}

// FromFS wraps the embedded corpus, whose paths start at "phrases".
func FromFS(fsys fs.FS) Corpus {
	return Corpus{fsys: fsys, root: "phrases", lang: DefaultLang}
}

// FromDir wraps a corpus on disk, overriding the embedded one.
func FromDir(dir string) Corpus {
	return Corpus{fsys: os.DirFS(dir), root: ".", lang: DefaultLang}
}

// In qualifies a path with the corpus language: In("states/worn.txt") is
// "en/states/worn.txt". Paths that are already language-free, like mine/, are
// used as they are.
func (c Corpus) In(rel string) string { return c.lang + "/" + rel }

// Lines reads every listed file and returns the phrases in them, in order.
//
// A corpus file is data on disk, and data on disk never gets to inject escapes
// into a prompt or a status row: anything non-printable is dropped, along with
// comments, blank lines and the optional trailing " -- @handle" attribution
// (which belongs to the contributor, not in the bird's mouth).
//
// A missing file is skipped rather than reported. Pools are assembled
// optimistically and half of them do not exist for any given state.
func (c Corpus) Lines(names ...string) []string {
	var out []string
	for _, name := range names {
		b, err := fs.ReadFile(c.fsys, path.Join(c.root, name))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(b), "\n") {
			if line = clean(line); line != "" {
				out = append(out, line)
			}
		}
	}
	return out
}

// Has reports whether a file exists and holds at least one usable phrase. An
// empty file must not win a pool and silence the bird.
func (c Corpus) Has(name string) bool {
	return len(c.Lines(name)) > 0
}

// Files lists the .txt files in a directory, sorted, as paths Lines accepts.
// Sorted because the order feeds a shuffle: an unstable order would change a
// pool's fingerprint on every run and reshuffle it forever.
func (c Corpus) Files(dir string) []string {
	entries, err := fs.ReadDir(c.fsys, path.Join(c.root, dir))
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".txt") {
			out = append(out, path.Join(dir, e.Name()))
		}
	}
	sort.Strings(out)
	return out
}

// All lists every phrase file in the corpus, sorted. The linter walks this; the
// bird never does.
func (c Corpus) All() []string {
	var out []string
	fs.WalkDir(c.fsys, c.root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() && strings.HasSuffix(p, ".txt") {
			rel := strings.TrimPrefix(p, c.root+"/")
			out = append(out, rel)
		}
		return nil
	})
	sort.Strings(out)
	return out
}

// Ephemeral returns the topical files still in date: this year's and last
// year's, and nothing else. The quarantine is the filename, so a line expires
// because of what it is called rather than because somebody reviewed it.
func (c Corpus) Ephemeral(now time.Time) []string {
	var out []string
	for y := now.Year() - EphemeralYears + 1; y <= now.Year(); y++ {
		if f := c.In("ephemeral/" + strconv.Itoa(y) + ".txt"); c.Has(f) {
			out = append(out, f)
		}
	}
	return out
}

func clean(line string) string {
	if i := strings.Index(line, "-- @"); i >= 0 && strings.TrimSpace(line[:i]) != "" {
		line = line[:i]
	}
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return ""
	}
	var b strings.Builder
	for _, r := range line {
		if unicode.IsPrint(r) {
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}
