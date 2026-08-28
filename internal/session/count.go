package session

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// MaxSessionsTracked bounds the file. Nobody opens thirty-two Claude Code
// sessions in a day, and if they do, the bird has already said everything it
// has to say about it.
const MaxSessionsTracked = 32

// CountToday records this session and returns how many distinct sessions today
// has seen.
//
// A set of session ids, not a counter: the status row is redrawn several times
// a second and cancelled mid-run, so anything incremented per refresh drifts
// within minutes. Recording an id twice is a no-op, which is the property that
// makes this safe on a path that runs constantly.
//
// The day is part of the file, so the count resets at midnight without anything
// having to expire it.
func CountToday(path, id string, today int) int {
	if id == "" {
		return 0 // shell mode has no session to count
	}
	day, ids := readSessions(path)
	if day != today {
		ids = nil
	}
	if !containsString(ids, id) {
		ids = append(ids, id)
		if len(ids) > MaxSessionsTracked {
			ids = ids[len(ids)-MaxSessionsTracked:]
		}
		writeSessions(path, today, ids)
	}
	return len(ids)
}

func readSessions(path string) (int, []string) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, nil
	}
	day := 0
	var ids []string
	for _, line := range strings.Split(string(raw), "\n") {
		k, v, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		switch k {
		case "day":
			day, _ = strconv.Atoi(strings.TrimSpace(v))
		case "ids":
			for _, id := range strings.Split(v, ",") {
				if id = sanitizeID(id); id != "" {
					ids = append(ids, id)
				}
			}
		}
	}
	return day, ids
}

func writeSessions(path string, today int, ids []string) {
	if path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	body := "day=" + strconv.Itoa(today) + "\nids=" + strings.Join(ids, ",") + "\n"
	_ = os.WriteFile(path, []byte(body), 0o644)
}

// sanitizeID keeps the characters a session id is made of and nothing else.
// This file is read back and joined into a status row eventually; nothing that
// comes off disk gets to carry an escape sequence there.
func sanitizeID(s string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		}
	}
	return b.String()
}

func containsString(hay []string, needle string) bool {
	for _, s := range hay {
		if s == needle {
			return true
		}
	}
	return false
}
