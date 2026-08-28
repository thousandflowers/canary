package history

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The awk block in canary-statusline.sh is the specification for debt, the
// personal baseline and the night streak. It is reproduced here verbatim so the
// Go is diffed against the real thing rather than against a paraphrase of it.
const awkOracle = `awk -v today="$1" -v dmax="$2" '
  { d[NR]=$1+0; p[NR]=$2+0; n=NR }
  END {
    debt=0; psum=0; pcnt=0;
    for (i=1;i<=n;i++) if (d[i] < today) {
      age = today - d[i]; v = p[i];
      for (k=0;k<age;k++) v = v/2;
      debt += v; psum += p[i]; pcnt++;
    }
    if (debt > dmax) debt = dmax;
    personal = (pcnt>0) ? int(psum/pcnt) : 0;
    nights=0; check=today-1; found=1;
    while (found) {
      found=0;
      for (i=1;i<=n;i++) if (d[i]==check && p[i]>=90) { found=1; break }
      if (found) { nights++; check-- }
    }
    printf "%d %d %d", int(debt), personal, nights;
  }' "$3"`

func runOracle(t *testing.T, file string, today, debtMax int) Summary {
	t.Helper()
	out, err := exec.Command("bash", "-c", awkOracle,
		"oracle", strconv.Itoa(today), strconv.Itoa(debtMax), file).Output()
	if err != nil {
		t.Fatalf("awk oracle failed: %v", err)
	}
	f := strings.Fields(string(out))
	if len(f) != 3 {
		t.Fatalf("awk oracle emitted %q, want 3 fields", out)
	}
	n := func(s string) int {
		v, err := strconv.Atoi(s)
		if err != nil {
			t.Fatalf("oracle field %q not an int: %v", s, err)
		}
		return v
	}
	return Summary{Debt: n(f[0]), Personal: n(f[1]), Nights: n(f[2])}
}

func writeHistory(t *testing.T, entries []Entry) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "history")
	var b strings.Builder
	for _, e := range entries {
		fmt.Fprintf(&b, "%d %d\n", e.Day, e.Peak)
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSummarizeMatchesAwk(t *testing.T) {
	const today = 20000
	cases := []struct {
		name    string
		entries []Entry
	}{
		{"empty", nil},
		{"single fresh day", []Entry{{today - 1, 40}}},
		// The case that makes the float arithmetic load-bearing: three aged
		// peaks whose halves each carry a fraction. Truncating per-day instead
		// of once at the end loses points here.
		{"fractional decay", []Entry{{today - 1, 95}, {today - 2, 95}, {today - 3, 95}}},
		{"odd peaks decaying", []Entry{{today - 1, 77}, {today - 2, 63}, {today - 3, 41}, {today - 4, 19}}},
		{"debt over the cap", []Entry{{today - 1, 100}, {today - 2, 100}, {today - 3, 100}}},
		{"streak of three", []Entry{{today - 1, 95}, {today - 2, 92}, {today - 3, 90}}},
		{"streak broken by a good day", []Entry{{today - 1, 95}, {today - 2, 40}, {today - 3, 95}}},
		{"today is excluded from its own debt", []Entry{{today, 100}, {today - 1, 50}}},
		{"a future day is ignored", []Entry{{today + 1, 100}, {today - 1, 50}}},
		{"ten days deep", []Entry{
			{today - 1, 91}, {today - 2, 92}, {today - 3, 93}, {today - 4, 94}, {today - 5, 95},
			{today - 6, 96}, {today - 7, 97}, {today - 8, 98}, {today - 9, 99}, {today - 10, 100},
		}},
		{"a gap in the middle", []Entry{{today - 1, 95}, {today - 5, 95}, {today - 9, 95}}},
		{"exactly at the streak threshold", []Entry{{today - 1, 90}, {today - 2, 89}}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := writeHistory(t, c.entries)
			want := runOracle(t, path, today, 30)

			got := Summarize(c.entries, today, 30)
			if got != want {
				t.Errorf("Summarize = %+v, awk says %+v", got, want)
			}
		})
	}
}

// The cap is applied to the float before truncation. Capping after int() would
// be off by a point whenever the uncapped debt has a fraction.
func TestDebtCapAppliedBeforeTruncation(t *testing.T) {
	const today = 20000
	entries := []Entry{{today - 1, 100}, {today - 2, 100}}
	for _, max := range []int{0, 1, 7, 30, 61, 100} {
		path := writeHistory(t, entries)
		want := runOracle(t, path, today, max)
		if got := Summarize(entries, today, max); got != want {
			t.Errorf("debtMax=%d: Summarize = %+v, awk says %+v", max, got, want)
		}
	}
}

func TestLoadMissingFileIsNotAnError(t *testing.T) {
	entries, err := Load(filepath.Join(t.TempDir(), "absent"))
	if err != nil {
		t.Fatalf("a first run has no history and must not error: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("got %d entries from a missing file", len(entries))
	}
}

func TestLoadSkipsCorruptLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history")
	if err := os.WriteFile(path, []byte("20000 50\ngarbage\n\n20001 not-a-number\n20002 60\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	// A corrupt line must not take the bird down with it.
	if len(entries) != 2 || entries[0].Peak != 50 || entries[1].Peak != 60 {
		t.Fatalf("got %+v, want the two well-formed lines", entries)
	}
}

// The statusline is killed mid-run routinely; the shell left history.tmp.NNNN
// files behind when that happened between write and rename.
func TestRecordPeakLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history")
	for i := range 5 {
		if err := RecordPeak(path, 20000+i, 50+i); err != nil {
			t.Fatal(err)
		}
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "*.tmp.*"))
	if len(matches) > 0 {
		t.Fatalf("temp files left behind: %v", matches)
	}
}

func TestRecordPeakKeepsTheHighestPeakForToday(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history")
	if err := RecordPeak(path, 20000, 80); err != nil {
		t.Fatal(err)
	}
	// A later, lower refresh must not walk the day's peak back down.
	if err := RecordPeak(path, 20000, 30); err != nil {
		t.Fatal(err)
	}
	entries, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Peak != 80 {
		t.Fatalf("got %+v, want a single day holding its peak of 80", entries)
	}
}

func TestRecordPeakPrunesToKeepDays(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history")
	for i := range Keep + 5 {
		if err := RecordPeak(path, 20000+i, 50); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != Keep {
		t.Fatalf("kept %d days, want %d", len(entries), Keep)
	}
	if entries[0].Day != 20005 {
		t.Errorf("pruned the wrong end: oldest kept day is %d, want 20005", entries[0].Day)
	}
}

func TestLoadReportsAPathItCannotStat(t *testing.T) {
	// A file where a directory should be. Not "missing", which is ordinary and
	// silent — broken, which the caller deserves to hear about.
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	os.WriteFile(blocker, nil, 0o644)

	if _, err := Load(filepath.Join(blocker, "history")); err == nil {
		t.Error("a path under a regular file should be an error")
	}
}

func TestLoadReportsAFileItCannotRead(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root; the mode would be ignored")
	}
	path := filepath.Join(t.TempDir(), "history")
	os.WriteFile(path, []byte("20692 40\n"), 0o000)

	if _, err := Load(path); err == nil {
		t.Error("an unreadable history should be an error, not an empty past")
	}
}

func TestRecordPeakRefusesASymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	link := filepath.Join(dir, "history")
	os.WriteFile(target, []byte("20692 40\n"), 0o644)
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if err := RecordPeak(link, 20693, 99); err != nil {
		t.Fatalf("RecordPeak should decline quietly, got %v", err)
	}
	if got, _ := os.ReadFile(target); string(got) != "20692 40\n" {
		t.Errorf("wrote through a symlink: %q", got)
	}
}

func TestRecordPeakStopsIfItCannotReadWhatIsThere(t *testing.T) {
	// Rewriting a history it could not read would throw away every day in it.
	if os.Geteuid() == 0 {
		t.Skip("running as root; the mode would be ignored")
	}
	path := filepath.Join(t.TempDir(), "history")
	os.WriteFile(path, []byte("20692 40\n"), 0o000)

	if err := RecordPeak(path, 20693, 99); err == nil {
		t.Error("recording onto an unreadable history should fail loudly")
	}
}

func TestRecordPeakReportsAWriteItCannotMake(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root; the mode would be ignored")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Skip(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o755) })

	if err := RecordPeak(filepath.Join(dir, "history"), 20693, 99); err == nil {
		t.Error("a failed write should be reported")
	}
}
