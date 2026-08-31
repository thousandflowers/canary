package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/thousandflowers/canary/internal/chrono"
	"github.com/thousandflowers/canary/internal/config"
)

// fakeKnowledge builds a knowledgeC-shaped database at the path macOS keeps one
// at, filled with `days` days of activity in each of `hours`.
//
// A real sqlite3 against a real file, in the style of the git fixtures in
// internal/session: the query is the interesting part, and a stub that returned
// rows would prove nothing about whether the SQL is right.
func fakeKnowledge(t *testing.T, home string, days int, hours []int) string {
	t.Helper()
	// sqlite3 reads the hours back through 'localtime'. Without pinning the
	// zone this fixture lands on different hours on every machine, which is a
	// flaky test dressed up as a real one.
	t.Setenv("TZ", "UTC")
	db := filepath.Join(home, knowledgeDB)
	if err := os.MkdirAll(filepath.Dir(db), 0o755); err != nil {
		t.Fatal(err)
	}

	var sql strings.Builder
	sql.WriteString("CREATE TABLE ZOBJECT (ZSTREAMNAME TEXT, ZSTARTDATE REAL);\n")
	// Core Data counts seconds from 2001-01-01, so day 9000 is a plausible
	// recent date and the offsets below land on real hours of it.
	for d := 0; d < days; d++ {
		for _, h := range hours {
			start := (9000+d)*86400 + h*3600
			sql.WriteString("INSERT INTO ZOBJECT VALUES ('/app/usage', ")
			sql.WriteString(strconv.Itoa(start))
			sql.WriteString(");\n")
		}
	}
	// A row the query must ignore: a different stream, and a null date.
	sql.WriteString("INSERT INTO ZOBJECT VALUES ('/app/inFocus', 777600000);\n")
	sql.WriteString("INSERT INTO ZOBJECT VALUES ('/app/usage', NULL);\n")

	cmd := exec.Command("sqlite3", db)
	cmd.Stdin = strings.NewReader(sql.String())
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("sqlite3 fixture: %v\n%s", err, out)
	}
	return db
}

func TestChronoSaysSoWhenItHasNothingYet(t *testing.T) {
	isolate(t)

	out, code := capture(t, func() int { return run([]string{"chrono"}) })
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if !strings.Contains(out, "Not enough shape yet") || !strings.Contains(out, "--bootstrap") {
		t.Errorf("an empty log should explain itself and point at the fix:\n%s", out)
	}
}

func TestChronoReportsALearnedOffset(t *testing.T) {
	home := isolate(t)
	cfg := config.FromEnv()
	// A night owl: up from noon to 03:00.
	var l chrono.Log
	for i := 0; i < 16; i++ {
		l.Slots[(12+i)%24] = chrono.SteadyState
	}
	if err := chrono.Save(cfg.ChronoFile, l); err != nil {
		t.Fatal(err)
	}
	_ = home

	out, code := capture(t, func() int { return run([]string{"chrono"}) })
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	for _, want := range []string{"Your day centres on", "Offset +5h", "deepest trough"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestChronoShowsAnOverrideBeatingWhatItLearned(t *testing.T) {
	isolate(t)
	t.Setenv("CANARY_CHRONO_OFFSET", "-2")
	cfg := config.FromEnv()
	var l chrono.Log
	for i := 0; i < 16; i++ {
		l.Slots[(12+i)%24] = chrono.SteadyState
	}
	if err := chrono.Save(cfg.ChronoFile, l); err != nil {
		t.Fatal(err)
	}

	out, _ := capture(t, func() int { return run([]string{"chrono"}) })
	if !strings.Contains(out, "overridden to -2h") {
		t.Errorf("an override must say it is overriding:\n%s", out)
	}
	if got := chronoOffset(cfg); got != -2 {
		t.Errorf("chronoOffset = %d, want -2", got)
	}
}

func TestChronoRejectsAnArgumentItDoesNotKnow(t *testing.T) {
	isolate(t)

	out, code := capture(t, func() int { return run([]string{"chrono", "--yesterday"}) })
	if code != 2 || !strings.Contains(out, "unknown argument") {
		t.Errorf("exit %d\n%s", code, out)
	}
}

func TestBootstrapSeedsFromScreenTimeHistory(t *testing.T) {
	home := isolate(t)
	// Thirty days of a night owl: awake noon to 03:00.
	hours := make([]int, 0, 16)
	for i := 0; i < 16; i++ {
		hours = append(hours, (12+i)%24)
	}
	fakeKnowledge(t, home, 30, hours)

	out, code := capture(t, func() int { return run([]string{"chrono", "--bootstrap"}) })
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if !strings.Contains(out, "Seeded from 30 days") {
		t.Errorf("no seed report:\n%s", out)
	}
	if !strings.Contains(out, "Offset +5h") {
		t.Errorf("seeded log did not produce the offset the data implies:\n%s", out)
	}

	// It must survive to disk, or the next prompt learns nothing.
	if off, ok := chrono.Load(config.FromEnv().ChronoFile).Offset(); !ok || off != 5 {
		t.Errorf("reloaded offset %d ok=%v, want 5 true", off, ok)
	}
}

func TestBootstrapWithoutAnyHistory(t *testing.T) {
	isolate(t)

	out, code := capture(t, func() int { return run([]string{"chrono", "--bootstrap"}) })
	if code != 1 || !strings.Contains(out, "no screen-time history") {
		t.Errorf("exit %d\n%s", code, out)
	}
}

func TestBootstrapRefusesTooLittleHistory(t *testing.T) {
	home := isolate(t)
	fakeKnowledge(t, home, 2, []int{14, 15}) // two days, two hours a day

	out, code := capture(t, func() int { return run([]string{"chrono", "--bootstrap"}) })
	if code != 1 || !strings.Contains(out, "not enough to seed from") {
		t.Errorf("exit %d\n%s", code, out)
	}
}

func TestBootstrapReportsADatabaseItCannotRead(t *testing.T) {
	home := isolate(t)
	db := filepath.Join(home, knowledgeDB)
	if err := os.MkdirAll(filepath.Dir(db), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(db, []byte("this is not a database"), 0o600); err != nil {
		t.Fatal(err)
	}

	out, code := capture(t, func() int { return run([]string{"chrono", "--bootstrap"}) })
	if code != 1 || !strings.Contains(out, "sqlite3") {
		t.Errorf("exit %d\n%s", code, out)
	}
	if !strings.Contains(out, "Full Disk Access") {
		t.Errorf("the likeliest cause on a Mac should be named:\n%s", out)
	}
}

// The counter file is written by canary and can be hand-edited, so the reader
// has to cope with a row that says something impossible.
func TestKnowledgeHoursSkipsRowsItCannotUse(t *testing.T) {
	home := isolate(t)
	db := filepath.Join(home, knowledgeDB)
	if err := os.MkdirAll(filepath.Dir(db), 0o755); err != nil {
		t.Fatal(err)
	}
	sql := `CREATE TABLE ZOBJECT (ZSTREAMNAME TEXT, ZSTARTDATE REAL);
INSERT INTO ZOBJECT VALUES ('/app/usage', 777600000);`
	cmd := exec.Command("sqlite3", db)
	cmd.Stdin = strings.NewReader(sql)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v\n%s", err, out)
	}

	counts, days, err := knowledgeHours(db)
	if err != nil {
		t.Fatalf("knowledgeHours: %v", err)
	}
	if days != 1 {
		t.Errorf("days = %d, want 1", days)
	}
	total := 0
	for _, n := range counts {
		total += n
	}
	if total != 1 {
		t.Errorf("total = %d, want 1", total)
	}
}

func TestSqliteReportsAMissingBinaryOrFile(t *testing.T) {
	if _, err := sqlite(filepath.Join(t.TempDir(), "nope.db"), "SELECT 1 FROM nothing;"); err == nil {
		t.Error("querying a table that is not there succeeded")
	}
}

func TestChronoChartDrawsWhatItHas(t *testing.T) {
	var l chrono.Log
	if chart := chronoChart(l); !strings.Contains(chart, "hours you are awake in") {
		t.Errorf("empty chart lost its caption:\n%s", chart)
	}

	l.Slots[3] = chrono.SteadyState
	l.Slots[15] = chrono.SteadyState / 2
	chart := chronoChart(l)
	if !strings.ContainsRune(chart, '█') {
		t.Errorf("the busiest hour should be a full block:\n%s", chart)
	}
}

func TestClockHourRendersFractions(t *testing.T) {
	for _, tc := range []struct {
		in   float64
		want string
	}{{0, "00:00"}, {13.5, "13:30"}, {19.75, "19:45"}, {23.999, "00:00"}} {
		if got := clockHour(tc.in); got != tc.want {
			t.Errorf("clockHour(%v) = %s, want %s", tc.in, got, tc.want)
		}
	}
}

// The rotation has to actually reach the score, or none of the above matters.
func TestTheLearnedOffsetReachesTheScore(t *testing.T) {
	isolate(t)
	cfg := config.FromEnv()

	plain := chronoOffset(cfg)
	if plain != 0 {
		t.Fatalf("a fresh machine must not rotate anything, got %d", plain)
	}

	var l chrono.Log
	for i := 0; i < 16; i++ {
		l.Slots[(12+i)%24] = chrono.SteadyState
	}
	if err := chrono.Save(cfg.ChronoFile, l); err != nil {
		t.Fatal(err)
	}
	if got := chronoOffset(cfg); got != 5 {
		t.Errorf("chronoOffset = %d, want 5", got)
	}
}

func TestSqliteReportsAMissingBinary(t *testing.T) {
	t.Setenv("PATH", "")

	_, err := sqlite(filepath.Join(t.TempDir(), "any.db"), "SELECT 1;")
	if err == nil || !strings.Contains(err.Error(), "sqlite3") {
		t.Errorf("err = %v, want one naming sqlite3", err)
	}
}

func TestBootstrapReportsASeedItCannotWrite(t *testing.T) {
	home := isolate(t)
	hours := make([]int, 0, 16)
	for i := 0; i < 16; i++ {
		hours = append(hours, (12+i)%24)
	}
	fakeKnowledge(t, home, 30, hours)

	// canary's own directory, made unwritable underneath it.
	dir := filepath.Join(home, ".canary")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	if os.Geteuid() == 0 {
		t.Skip("root writes anywhere")
	}

	out, code := capture(t, func() int { return run([]string{"chrono", "--bootstrap"}) })
	if code != 1 {
		t.Errorf("exit %d, want 1\n%s", code, out)
	}
}

func TestParseKnowledgeSurvivesRowsSqliteWouldNotEmit(t *testing.T) {
	counts, days := parseKnowledge([]string{
		"h|13|17",  // the good case
		"d|30|0",   // the day count
		"h|9",      // truncated
		"",         // blank
		"h|x|4",    // hour is not a number
		"h|4|y",    // count is not a number
		"h|25|9",   // no such hour
		"h|-1|9",   // nor that one
		"junk|1|1", // a tag from a query that no longer exists
	})

	if days != 30 {
		t.Errorf("days = %d, want 30", days)
	}
	if counts[13] != 17 {
		t.Errorf("counts[13] = %d, want 17", counts[13])
	}
	total := 0
	for _, n := range counts {
		total += n
	}
	if total != 17 {
		t.Errorf("total = %d, want 17 — a bad row leaked in", total)
	}
}
