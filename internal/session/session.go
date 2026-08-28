// Package session gathers the fatigue signals, from whichever of the two
// worlds the bird finds itself in.
//
//   - Claude Code mode: Claude Code pipes its session JSON on stdin every
//     refresh. Session minutes come from cost.total_duration_ms, and the richer
//     signals from walking the transcript JSONL it points at.
//   - Shell mode: nothing was piped in, so fall back to the state file the
//     shell-prompt bird refreshes on every command.
package session

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strconv"
	"strings"
)

// TranscriptTail is how much of the transcript is scanned, in bytes.
//
// On long sessions the transcript grows to tens of MB, and scanning all of it
// several times per refresh blew Claude Code's statusline timeout, so NOTHING
// rendered — the bird went invisible in exactly the long sessions it exists to
// warn about. Capping the read holds the work near-constant regardless of file
// size. Older lines cannot change the band anyway: a session that long is
// already maxed on minutes, so the signals reflect recent activity, which is
// what fatigue should weight.
const TranscriptTail = 2 << 20 // 2 MiB

// MaxReps caps the repetition term so a polling or retry loop cannot kill the
// bird on its own.
const MaxReps = 5

// SameFilePasses is how many times one file has to be touched before the bird
// notices. Three is ordinary work — read, edit, fix. The fourth pass is the one
// that starts to look like an argument with a file.
const SameFilePasses = 4

// MinFilesForNoTests keeps the no-tests trigger off a session that has barely
// touched anything: two files and no test file is not a finding.
const MinFilesForNoTests = 3

// Signals is what a mode managed to observe.
type Signals struct {
	Minutes int
	Turns   int // human turns in CC mode, prompts in shell mode
	Errors  int // failed tool calls
	Reps    int // extra repeats of one command, beyond the first
	// StatName labels Turns in the rendered stat line: t for turns, p for
	// prompts. The two are not the same measurement and the row should not
	// pretend they are.
	StatName string
	// AvgLen is the mean command length; shell mode only, where there is no
	// error or repetition signal to lean on instead.
	AvgLen int

	// SessionID identifies the Claude Code session. It is what "never twice in
	// a session" is measured against, and what counts the sessions in a day.
	SessionID string
	// Dir is the directory Claude Code is working in, which is where the
	// working-tree check has to run.
	Dir string

	// The rest are what the triggers are made of. They describe the session,
	// not the person: `same-file` is a fact about a file being reopened, and
	// the bird only ever reports facts.
	TopFile      string // the path touched most often
	TopFileCount int
	Files        int  // distinct paths touched
	HasTests     bool // any of them looked like a test
	Compacted    bool // the session has been compacted at least once
	Interrupted  bool // you stopped it mid-answer
	RepeatedAsk  bool // the same prompt, twice
}

// statusInput is the slice of Claude Code's session JSON the bird reads.
type statusInput struct {
	Cost struct {
		TotalDurationMS int64 `json:"total_duration_ms"`
	} `json:"cost"`
	TranscriptPath string `json:"transcript_path"`
	SessionID      string `json:"session_id"`
	CWD            string `json:"cwd"`
	Workspace      struct {
		CurrentDir string `json:"current_dir"`
	} `json:"workspace"`
}

// IsClaudeCode reports whether the piped bytes are Claude Code's session JSON.
// The transcript path is the discriminator the shell used, and it stays the
// discriminator: anything else on stdin means shell mode.
func IsClaudeCode(input []byte) bool {
	return bytes.Contains(input, []byte(`"transcript_path"`))
}

// FromClaudeCode reads the session JSON and, when it points at a usable
// transcript, the signals inside it.
//
// A malformed payload yields zero signals rather than an error: the statusline
// runs on every refresh and a bad frame should make the bird quiet, never make
// it crash into Claude Code's status row.
func FromClaudeCode(input []byte) Signals {
	sig := Signals{StatName: "t"}

	var in statusInput
	if err := json.Unmarshal(input, &in); err != nil {
		return sig
	}
	sig.Minutes = int(in.Cost.TotalDurationMS / 60000)
	sig.SessionID = in.SessionID
	sig.Dir = in.Workspace.CurrentDir
	if sig.Dir == "" {
		sig.Dir = in.CWD
	}

	f, ok := openTranscript(in.TranscriptPath)
	if !ok {
		return sig
	}
	defer f.Close()

	scanTranscript(f, &sig)
	return sig
}

// openTranscript opens the transcript, seeked to the tail, or reports that
// there is nothing usable to read.
//
// Symlinks are refused: the path arrives from outside the process, and
// following a link would let it point the read anywhere.
func openTranscript(path string) (io.ReadCloser, bool) {
	if path == "" {
		return nil, false
	}
	fi, err := os.Lstat(path)
	if err != nil || fi.Mode()&os.ModeSymlink != 0 || !fi.Mode().IsRegular() {
		return nil, false
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	if fi.Size() > TranscriptTail {
		if _, err := f.Seek(fi.Size()-TranscriptTail, io.SeekStart); err != nil {
			f.Close()
			return nil, false
		}
		// The seek lands mid-line. Unlike `tail -c`, drop that fragment rather
		// than counting it: a partial JSONL line is not a turn, and half a
		// record matching a pattern by accident is a miscount, not a signal.
		br := bufio.NewReader(f)
		if _, err := br.ReadString('\n'); err != nil {
			f.Close()
			return nil, false
		}
		return readCloser{Reader: br, Closer: f}, true
	}
	return f, true
}

type readCloser struct {
	io.Reader
	io.Closer
}

// scanTranscript walks the JSONL once, filling in everything that can be read
// from it. The shell needed four greps over the same buffer; one pass is both
// faster and the only way to see runs of repeated commands in order.
//
// This is on the hot path — Claude Code redraws the status row several times a
// second and cancels the previous draw — so everything here is a substring
// scan over bytes. Unmarshalling every line of a 2 MiB tail would be the one
// change that makes the bird disappear in long sessions again.
func scanTranscript(r io.Reader, sig *Signals) {
	sc := bufio.NewScanner(r)
	// JSONL lines carry whole tool results and routinely exceed the default
	// 64KB limit. A truncated line would silently stop the scan.
	sc.Buffer(make([]byte, 0, 256<<10), 16<<20)

	var lastCmd string
	run := 0
	maxRun := 0
	files := map[string]int{}
	asks := map[string]int{}

	for sc.Scan() {
		line := sc.Bytes()

		// A human turn is a "type":"user" line that is NOT a tool result:
		// Claude Code wraps each tool result as its own "type":"user" line.
		// Excluding tool_result lines directly is robust; subtracting one count
		// from the other underflowed to zero.
		if bytes.Contains(line, []byte(`"type":"user"`)) &&
			!bytes.Contains(line, []byte("tool_result")) {
			sig.Turns++
			// The same question asked twice is not a longer session, it is a
			// stuck one. Compared on a prefix: the tail of a long prompt is
			// usually where the edits are.
			if ask := firstAsk(line); ask != "" {
				asks[ask]++
				if asks[ask] > 1 {
					sig.RepeatedAsk = true
				}
			}
		}
		if bytes.Contains(line, []byte(`"is_error":true`)) {
			sig.Errors++
		}

		// A compaction means the session outlived its own context at least
		// once, which is a fact about how long this has been going on.
		if bytes.Contains(line, []byte(`"isCompactSummary":true`)) ||
			bytes.Contains(line, []byte(`"compact_boundary"`)) {
			sig.Compacted = true
		}
		if bytes.Contains(line, []byte("[Request interrupted by")) {
			sig.Interrupted = true
		}

		// Every file the session touched, and how often. One file coming back
		// again and again is the `same-file` trigger; a session with files but
		// no test file among them is `no-tests`.
		for _, p := range valuesOf(line, `"file_path":"`) {
			files[p]++
			if isTestPath(p) {
				sig.HasTests = true
			}
		}

		// Perseveration: the longest run of the same command back-to-back.
		// Mental fatigue reliably increases both error rates and the repeating
		// of an action that is not working, which is what this counts.
		for _, cmd := range commandsIn(line) {
			if cmd == lastCmd {
				run++
			} else {
				lastCmd, run = cmd, 1
			}
			if run > maxRun {
				maxRun = run
			}
		}
	}

	if maxRun > 1 {
		sig.Reps = maxRun - 1
	}
	if sig.Reps > MaxReps {
		sig.Reps = MaxReps
	}

	sig.Files = len(files)
	for p, n := range files {
		// Ties go to the longer path, so the winner is stable rather than
		// whichever key the map happened to hand back first.
		if n > sig.TopFileCount || (n == sig.TopFileCount && p > sig.TopFile) {
			sig.TopFile, sig.TopFileCount = p, n
		}
	}
}

// isTestPath is deliberately a spelling check, not a language one: the bird is
// not going to parse anyone's build system from a status line.
func isTestPath(p string) bool {
	// Leading separator added so a directory marker matches at the start of a
	// relative path too: Claude Code reports both "/repo/tests/e2e.py" and
	// "tests/e2e.py", and only one of them has a slash in front of "tests".
	p = "/" + strings.TrimPrefix(strings.ToLower(p), "/")
	for _, mark := range []string{"_test.", "test_", ".test.", "_spec.", ".spec.", "/tests/", "/test/", "/spec/"} {
		if strings.Contains(p, mark) {
			return true
		}
	}
	return false
}

// firstAsk returns a comparable prefix of a human turn's text.
func firstAsk(line []byte) string {
	const key = `"content":"`
	i := bytes.Index(line, []byte(key))
	if i < 0 {
		return ""
	}
	rest := line[i+len(key):]
	end := 0
	for end < len(rest) {
		if rest[end] == '\\' {
			end += 2
			continue
		}
		if rest[end] == '"' {
			break
		}
		end++
	}
	if end > len(rest) {
		end = len(rest)
	}
	ask := strings.TrimSpace(strings.ToLower(string(rest[:end])))
	if len(ask) < 12 {
		return "" // "yes", "go on" and "ok" repeat all day and mean nothing
	}
	if len(ask) > 80 {
		ask = ask[:80]
	}
	return ask
}

// valuesOf extracts every value for a JSON string key on one line, in order.
func valuesOf(line []byte, key string) []string {
	var out []string
	k := []byte(key)
	for {
		i := bytes.Index(line, k)
		if i < 0 {
			return out
		}
		line = line[i+len(k):]
		end := 0
		for end < len(line) {
			if line[end] == '\\' {
				end += 2
				continue
			}
			if line[end] == '"' {
				break
			}
			end++
		}
		if end > len(line) {
			return out
		}
		out = append(out, string(line[:end]))
		if end >= len(line) {
			return out
		}
		line = line[end:]
	}
}

// commandsIn extracts every "command":"..." value on a line, in order.
func commandsIn(line []byte) []string { return valuesOf(line, `"command":"`) }

// FromShellState reads the state file the shell-prompt bird writes.
//
// ok is false when there is no usable state, which the caller must treat as
// "say nothing": a statusline with no session and no state has nothing true to
// report, and a fresh bird there would be a lie.
func FromShellState(path string) (Signals, bool) {
	fi, err := os.Lstat(path)
	if err != nil || fi.Mode()&os.ModeSymlink != 0 || !fi.Mode().IsRegular() {
		return Signals{}, false
	}
	f, err := os.Open(path)
	if err != nil {
		return Signals{}, false
	}
	defer f.Close()

	var promptCount, avgLen, active int
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, found := strings.Cut(sc.Text(), "=")
		if !found {
			continue
		}
		// A field is a number or it is nothing. Squeezing the digits out of
		// whatever is there (the shell's `tr -cd '0-9'`) turns an injected
		// "\x1b[31m9" into 319 — garbage in, a plausible number out.
		n, ok := parseInt(v)
		if !ok {
			continue
		}
		switch k {
		case "prompt_count":
			promptCount = n
		case "avg_prompt_len":
			avgLen = n
		case "active_seconds":
			active = n
		}
	}

	return Signals{
		Minutes:  active / 60,
		Turns:    promptCount,
		AvgLen:   avgLen,
		StatName: "p",
	}, true
}

// parseInt accepts a plain non-negative integer and nothing else.
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
