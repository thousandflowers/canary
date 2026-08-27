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
	"strings"
	"unicode"
)

// Corpus is a phrase tree rooted at its `en` directory.
type Corpus struct {
	fsys fs.FS
	root string
}

// FromFS wraps the embedded corpus, whose paths start at "phrases".
func FromFS(fsys fs.FS) Corpus {
	return Corpus{fsys: fsys, root: "phrases/en"}
}

// FromDir wraps a corpus on disk, overriding the embedded one.
func FromDir(dir string) Corpus {
	return Corpus{fsys: os.DirFS(dir), root: "en"}
}

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
