package phrase

import (
	"encoding/json"
	"hash/fnv"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// A bag is sampling without replacement: shuffle the pool, consume it in
// order, reshuffle only on exhaustion.
//
// A plain rand() over thirty lines repeats within about seven draws, and the
// rarity dies the first time somebody sees the same "rare" line twice in an
// evening. The recent queue alone could not fix that — it only ever removed
// the most visible symptom, the same line twice running.
//
// The state is persisted because the alternative is a bag that resets every
// time Claude Code redraws the status row, which is several times a second.
type Bag struct {
	path  string
	pools map[string]*poolState
	dirty bool
}

// poolState is one pool's permutation and how far through it we are.
//
// Indices, not the lines themselves: the file is rewritten constantly and a
// copy of the corpus in it would be both large and stale. The fingerprint
// catches the corpus changing underneath — an upgrade, or a contributor
// editing phrases/ — and reshuffles rather than handing out the wrong line.
type poolState struct {
	Fingerprint string `json:"fp"`
	Order       []int  `json:"order"`
	Next        int    `json:"next"`
}

// LoadBag reads the shuffle state. A missing, corrupt or symlinked file yields
// an empty bag: every pool reshuffles once and nothing else is affected, which
// is a far better failure than refusing to draw.
func LoadBag(path string) *Bag {
	b := &Bag{path: path, pools: map[string]*poolState{}}
	fi, err := os.Lstat(path)
	if err != nil || fi.Mode()&os.ModeSymlink != 0 || !fi.Mode().IsRegular() {
		return b
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return b
	}
	var pools map[string]*poolState
	if err := json.Unmarshal(raw, &pools); err != nil {
		return b
	}
	for k, v := range pools {
		if v != nil {
			b.pools[k] = v
		}
	}
	return b
}

// Draw takes the next line from a pool, skipping anything in recent.
//
// key names the pool — the files it was assembled from — so `states/tired.txt`
// keeps its own place in its own shuffle no matter what else was drawn in
// between.
func (b *Bag) Draw(key string, lines []string, r Rand, recent []string) string {
	if len(lines) == 0 {
		return ""
	}
	fp := fingerprint(lines)
	st := b.pools[key]
	if st == nil || st.Fingerprint != fp || len(st.Order) != len(lines) {
		st = &poolState{Fingerprint: fp, Order: shuffled(len(lines), r)}
		b.pools[key] = st
		b.dirty = true
	}

	// Two full cycles at most. One is not enough: a reshuffle landing in the
	// middle of the walk can deal the excluded lines twice in a row, and the
	// bag would give up while a usable line was still sitting in the pool. Two
	// cycles guarantee every line is offered at least once. A pool small enough
	// to sit entirely inside the recent queue falls through to the fallback,
	// because a bird that cannot repeat itself is a bird that never speaks.
	var fallback string
	for tries := 0; tries < 2*len(lines); tries++ {
		if st.Next >= len(st.Order) {
			// Exhausted: reshuffle. The recent queue still applies, which is
			// what stops the tail of one cycle reappearing at the head of the
			// next — the most visible failure mode there is.
			st.Order = shuffled(len(lines), r)
			st.Next = 0
		}
		line := lines[st.Order[st.Next]]
		st.Next++
		b.dirty = true
		if fallback == "" {
			fallback = line
		}
		if !contains(recent, line) {
			return line
		}
	}
	return fallback
}

// Save writes the bag if anything changed. Writing on every refresh when
// nothing was drawn would be several file writes a second for no reason.
func (b *Bag) Save() error {
	if b == nil || !b.dirty || b.path == "" {
		return nil
	}
	raw, err := json.Marshal(b.pools)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(b.path), 0o755); err != nil {
		return err
	}
	return writeAtomic(b.path, string(raw))
}

// shuffled is a Fisher-Yates permutation of 0..n-1.
func shuffled(n int, r Rand) []int {
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	for i := n - 1; i > 0; i-- {
		j := r.IntN(i + 1)
		order[i], order[j] = order[j], order[i]
	}
	return order
}

// fingerprint identifies a pool's contents cheaply. A hash, not the lines: the
// only question it has to answer is "did this change".
func fingerprint(lines []string) string {
	h := fnv.New64a()
	for _, l := range lines {
		h.Write([]byte(l))
		h.Write([]byte{'\n'})
	}
	return strconv.FormatUint(h.Sum64(), 36)
}

// poolKey names a pool by the files it came from, so the same combination of
// files always resumes the same shuffle.
func poolKey(files []string) string { return strings.Join(files, "|") }
