package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/thousandflowers/canary/internal/config"
)

// Claude Code allows exactly one statusLine command, so canary appends itself
// to whatever is already there (caveman's [CAVEMAN] badge, most often) instead
// of replacing it. caveman prints no trailing newline and neither does canary's
// last line, so the bird lands right beside the badge.
//
// This is the whole reason `jq` used to be a dependency of the installer. A
// binary that already speaks JSON does not need to shell out to another one.
const settingsMarker = "canary"

func runSettings(cfg config.Config, args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: canary settings install|remove")
		return 2
	}
	path := settingsPath(cfg)
	switch args[0] {
	case "install":
		return settingsInstall(path)
	case "remove":
		return settingsRemove(path)
	default:
		fmt.Fprintln(os.Stderr, "usage: canary settings install|remove")
		return 2
	}
}

// settingsPath honours CLAUDE_CONFIG_DIR, which is how people move the Claude
// Code config off ~/.claude.
func settingsPath(cfg config.Config) string {
	dir := os.Getenv("CLAUDE_CONFIG_DIR")
	if dir == "" {
		dir = filepath.Join(cfg.Home, ".claude")
	}
	return filepath.Join(dir, "settings.json")
}

func settingsInstall(path string) int {
	add := statuslineCommand()

	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		// Claude Code is not installed here, which is not a failure: plenty of
		// people only ever want the shell bird.
		fmt.Printf("canary: no Claude Code config at %s — skipping the status line\n", filepath.Dir(path))
		return 0
	}

	settings, ok := readSettings(path)
	if !ok {
		fmt.Printf("canary: %s is not plain JSON (comments?) — left untouched.\n", path)
		fmt.Printf("        add this to statusLine.command yourself: %s\n", add)
		return 0
	}

	cur := currentCommand(settings)
	if strings.Contains(cur, settingsMarker) {
		fmt.Println("canary: status line already wired")
		return 0
	}
	newCmd := add
	if cur != "" {
		newCmd = cur + "; " + add
	}

	// Back up before the first write. The file holds the person's own Claude
	// Code configuration, and canary is a toy bird — it does not get to be the
	// reason that file is unrecoverable.
	if raw, err := os.ReadFile(path); err == nil {
		_ = os.WriteFile(path+".canary.bak", raw, 0o600)
	}

	settings["statusLine"] = map[string]any{"type": "command", "command": newCmd}
	if err := writeSettings(path, settings); err != nil {
		return fail(err)
	}
	fmt.Printf("canary: status line wired into %s (backup: %s.canary.bak)\n", path, path)
	return 0
}

func settingsRemove(path string) int {
	settings, ok := readSettings(path)
	if !ok {
		return 0 // nothing to unwire, or nothing safe to touch
	}
	cur := currentCommand(settings)
	if cur == "" {
		return 0
	}

	// Remove only canary's own segment. Whatever shares the row was there
	// first and must survive the uninstall.
	var kept []string
	for _, seg := range strings.Split(cur, ";") {
		if !strings.Contains(seg, settingsMarker) {
			if s := strings.TrimSpace(seg); s != "" {
				kept = append(kept, s)
			}
		}
	}

	if len(kept) == 0 {
		delete(settings, "statusLine")
	} else {
		settings["statusLine"] = map[string]any{"type": "command", "command": strings.Join(kept, "; ")}
	}
	if err := writeSettings(path, settings); err != nil {
		return fail(err)
	}
	fmt.Printf("canary: status line removed from %s\n", path)
	return 0
}

// statuslineCommand is the absolute path to this binary plus its subcommand.
// Absolute because Claude Code's PATH is not the shell's, and a bare `canary`
// there resolves for some people and silently does not for others.
func statuslineCommand() string {
	exe, err := os.Executable()
	if err != nil {
		exe = "canary" // PATH is the only fallback left
	} else if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved // Homebrew installs a symlink into its bin
	}
	return shellQuote(exe) + " statusline"
}

// readSettings parses the file, treating a missing one as empty. ok is false
// when the file exists but is not JSON canary can safely rewrite — JSONC with
// comments, most likely — because rewriting it would silently drop them.
func readSettings(path string) (map[string]any, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, true
		}
		return nil, false
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return map[string]any{}, true
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, false
	}
	return out, true
}

// writeSettings replaces the file atomically, through any symlink and with the
// mode it already had.
//
// settings.json is very often a symlink into a dotfiles repo. Renaming over the
// link would replace it with a fresh regular file: the dotfiles copy would keep
// the old configuration, the repo would silently detach, and the person would
// find out weeks later.
func writeSettings(path string, settings map[string]any) error {
	b, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')

	mode := os.FileMode(0o644)
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
		if fi, err := os.Stat(path); err == nil {
			mode = fi.Mode().Perm()
		}
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "settings.json.canary.*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// currentCommand digs statusLine.command out of the settings, tolerating any
// shape: the key may be missing, a string, or something else entirely.
func currentCommand(settings map[string]any) string {
	sl, ok := settings["statusLine"].(map[string]any)
	if !ok {
		return ""
	}
	cmd, _ := sl["command"].(string)
	return cmd
}

// shellQuote wraps a path for the shell Claude Code runs the command in. Paths
// with spaces are ordinary on macOS, and one unquoted space turns the status
// line into a command not found.
func shellQuote(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "`", "\\`", `$`, `\$`)
	return `"` + r.Replace(s) + `"`
}
