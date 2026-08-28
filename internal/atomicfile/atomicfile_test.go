package atomicfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteCreatesTheDirectoryAndTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "state")
	if err := Write(path, []byte("hello\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "hello\n" {
		t.Errorf("read back %q, %v", got, err)
	}
	fi, err := os.Stat(path)
	if err != nil || fi.Mode().Perm() != Mode {
		t.Errorf("mode = %v, want %v", fi.Mode().Perm(), os.FileMode(Mode))
	}
}

func TestWriteReplacesWhatWasThere(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state")
	Write(path, []byte("first"))
	if err := Write(path, []byte("second")); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "second" {
		t.Errorf("got %q", got)
	}
}

func TestWriteLeavesNoTempFileBehind(t *testing.T) {
	// The shell left `*.tmp.NNNN` litter in ~/.canary every time Claude Code
	// killed it between the write and the rename.
	dir := t.TempDir()
	if err := Write(filepath.Join(dir, "state"), []byte("x")); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp.") {
			t.Errorf("left behind: %s", e.Name())
		}
	}
}

func TestWriteRefusesASymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	link := filepath.Join(dir, "link")
	os.WriteFile(target, []byte("original"), 0o644)
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if err := Write(link, []byte("through the link")); err != nil {
		t.Fatalf("Write should decline quietly, got %v", err)
	}
	if got, _ := os.ReadFile(target); string(got) != "original" {
		t.Errorf("wrote through a symlink: %q", got)
	}
}

func TestWriteReportsADirectoryItCannotMake(t *testing.T) {
	// A file where a directory should be: MkdirAll cannot win this.
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	os.WriteFile(blocker, nil, 0o644)

	if err := Write(filepath.Join(blocker, "nested", "state"), []byte("x")); err == nil {
		t.Error("writing under a regular file should fail")
	}
}

func TestWriteReportsAFileItCannotCreate(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root; the mode would be ignored")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Skip(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o755) })

	if err := Write(filepath.Join(dir, "state"), []byte("x")); err == nil {
		t.Error("writing into a read-only directory should fail")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("a temp file survived the failure: %v", entries)
	}
}

func TestWriteReportsARenameItCannotMake(t *testing.T) {
	// A directory in the way of the destination: the temp file is written, and
	// the rename is what fails.
	dir := t.TempDir()
	path := filepath.Join(dir, "state")
	os.MkdirAll(filepath.Join(path, "occupied"), 0o755)

	if err := Write(path, []byte("x")); err == nil {
		t.Error("renaming over a non-empty directory should fail")
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp.") {
			t.Errorf("the temp file survived a failed rename: %s", e.Name())
		}
	}
}
