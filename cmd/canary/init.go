package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// The guard tests the binary itself, not `command -v canary`: a fresh curl
// install has the binary in ~/.local/bin before that directory is on PATH, and
// a hook that waited for PATH would leave the first shell birdless.
//
// The shell hooks are all that is left of canary.sh, canary.fish and
// canary-statusline.sh: two calls into the binary, per shell. Everything they
// used to do — the score, the art, the state file, the phrases — moved into Go,
// where the three of them can no longer drift apart.
//
// `canary init <shell>` prints the snippet rather than the installer writing a
// copy of it into ~/.canary: an upgrade then ships the new hook with the
// binary, instead of leaving a stale script behind that still works well enough
// that nobody notices.
const zshHook = `# canary — the fatigue bird. https://github.com/thousandflowers/canary
if [ -x {{canary}} ] && [ -z "${_CANARY_LOADED:-}" ]; then
  _CANARY_LOADED=1
  autoload -Uz add-zsh-hook
  _canary_preexec() { {{canary}} record -- "$1" }
  _canary_precmd()  { {{canary}} prompt }
  add-zsh-hook preexec _canary_preexec
  add-zsh-hook precmd  _canary_precmd
fi`

const bashHook = `# canary — the fatigue bird. https://github.com/thousandflowers/canary
if [ -x {{canary}} ] && [ -z "${_CANARY_LOADED:-}" ]; then
  _CANARY_LOADED=1
  # bash has no preexec. The DEBUG trap is the usual stand-in, gated by a
  # once-per-prompt flag so a command is recorded once, not once per word.
  _canary_debug() {
    [ -n "${_CANARY_AT_PROMPT:-}" ] || return 0
    _CANARY_AT_PROMPT=""
    {{canary}} record -- "$BASH_COMMAND"
  }
  trap '_canary_debug' DEBUG
  _canary_precmd() { _CANARY_AT_PROMPT=1; {{canary}} prompt; }
  # Do not clobber an existing PROMPT_COMMAND — it is usually someone's prompt.
  case "${PROMPT_COMMAND:-}" in
    *_canary_precmd*) : ;;
    "")  PROMPT_COMMAND="_canary_precmd" ;;
    *)   PROMPT_COMMAND="_canary_precmd; $PROMPT_COMMAND" ;;
  esac
fi`

const fishHook = `# canary — the fatigue bird. https://github.com/thousandflowers/canary
if status is-interactive; and test -x {{canary}}; and not set -q _CANARY_LOADED
    set -g _CANARY_LOADED 1

    function _canary_record --on-event fish_preexec
        {{canary}} record -- $argv
    end

    # Wrap the existing prompt exactly once. Copying an already-wrapped
    # fish_prompt into _canary_user_prompt would recurse forever.
    if functions -q fish_prompt
        functions -c fish_prompt _canary_user_prompt
    end
    function fish_prompt
        {{canary}} prompt
        if functions -q _canary_user_prompt
            _canary_user_prompt
        else
            printf '%s> ' (prompt_pwd)
        end
    end
end`

// withBinary points the hook at the binary that printed it, by absolute path.
//
// The rc file evaluates this snippet on every shell start, so the hook is
// always the one that shipped with the installed binary — no stale copy in
// ~/.canary quietly doing the old thing. The absolute path also means the hook
// keeps working when ~/.local/bin is not on PATH yet, which is the state a
// brand new install is in until the shell is restarted.
func withBinary(hook string) string {
	return strings.ReplaceAll(hook, "{{canary}}", shellQuote(binaryPath())) + "\n"
}

// binaryPath resolves this executable, following the symlink Homebrew installs.
func binaryPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "canary" // PATH is the only fallback left
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		return resolved
	}
	return exe
}

func runInit(args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: canary init zsh|bash|fish")
		return 2
	}
	// Written, not Println'd: the fish hook contains a printf format string,
	// and go vet is right to refuse to guess whether that was a mistake.
	switch args[0] {
	case "zsh":
		os.Stdout.WriteString(withBinary(zshHook))
	case "bash":
		os.Stdout.WriteString(withBinary(bashHook))
	case "fish":
		os.Stdout.WriteString(withBinary(fishHook))
	default:
		fmt.Fprintf(os.Stderr, "canary: no hook for %s (zsh, bash and fish are supported)\n", args[0])
		return 2
	}
	return 0
}
