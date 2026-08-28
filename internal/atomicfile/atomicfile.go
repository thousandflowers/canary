// Package atomicfile writes the small state files under ~/.canary.
//
// There were three copies of this: history, the phrase memory and the shell
// state each had their own temp-file-and-rename, and each had its own set of
// error branches to get wrong. Every file canary writes is small, is rewritten
// constantly, and must never be found half-written by the next refresh — one
// implementation, one set of rules.
package atomicfile

import (
	"os"
	"path/filepath"
	"strconv"
)

// Mode is what canary's own files are created with. Nobody else needs to read
// how tired you were last Tuesday.
const Mode = 0o600

// Write replaces path with data, atomically, and refuses to follow a symlink.
//
// The refusal matters because these paths are rewritten on every command and on
// every status-row refresh: without it, anything that can write to ~/.canary
// could point one of them somewhere else and have canary do the writing.
//
// The temp file carries the pid so two shells writing at once cannot land on
// the same name, and it is removed on every failure — the shell version left
// `*.tmp.NNNN` litter behind every time Claude Code killed it between the write
// and the rename.
func Write(path string, data []byte) error {
	if fi, err := os.Lstat(path); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		return nil
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	tmp := path + ".tmp." + strconv.Itoa(os.Getpid())
	if err := os.WriteFile(tmp, data, Mode); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
