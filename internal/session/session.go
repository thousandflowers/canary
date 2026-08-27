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
}

// statusInput is the slice of Claude Code's session JSON the bird reads.
type statusInput struct {
	Cost struct {
		TotalDurationMS int64 `json:"total_duration_ms"`
	} `json:"cost"`
	TranscriptPath string `json:"transcript_path"`
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

	f, ok := openTranscript(in.TranscriptPath)
	if !ok {
		return sig
	}
	defer f.Close()

	turns, errs, reps := scanTranscript(f)
	sig.Turns, sig.Errors, sig.Reps = turns, errs, reps
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

// scanTranscript walks the JSONL once, counting all three signals in a single
// pass. The shell needed four greps over the same buffer; one pass is both
// faster and the only way to see runs of repeated commands in order.
func scanTranscript(r io.Reader) (turns, errors, reps int) {
	sc := bufio.NewScanner(r)
	// JSONL lines carry whole tool results and routinely exceed the default
	// 64KB limit. A truncated line would silently stop the scan.
	sc.Buffer(make([]byte, 0, 256<<10), 16<<20)

	var lastCmd string
	run := 0
	maxRun := 0

	for sc.Scan() {
		line := sc.Bytes()

		// A human turn is a "type":"user" line that is NOT a tool result:
		// Claude Code wraps each tool result as its own "type":"user" line.
		// Excluding tool_result lines directly is robust; subtracting one count
		// from the other underflowed to zero.
		if bytes.Contains(line, []byte(`"type":"user"`)) &&
			!bytes.Contains(line, []byte("tool_result")) {
			turns++
		}
		if bytes.Contains(line, []byte(`"is_error":true`)) {
			errors++
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
		reps = maxRun - 1
	}
	if reps > MaxReps {
		reps = MaxReps
	}
	return turns, errors, reps
}

// commandsIn extracts every "command":"..." value on a line, in order.
func commandsIn(line []byte) []string {
	const key = `"command":"`
	var out []string
	for {
		i := bytes.Index(line, []byte(key))
		if i < 0 {
			return out
		}
		line = line[i+len(key):]
		// The value ends at the first unescaped quote.
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
