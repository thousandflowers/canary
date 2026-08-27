// Command canary is the fatigue bird: one binary for the shell prompt, for
// Claude Code's status row, and for the installer's wiring.
//
// It replaces canary.sh, canary.fish and canary-statusline.sh, which were three
// implementations of one formula kept in step by a test suite that asserted
// their agreement. One implementation cannot disagree with itself, the corpus
// ships inside the binary instead of being copied into ~/.canary by the
// packaging, and jq is no longer required to wire the status line.
package main

import (
	"fmt"
	"os"

	"github.com/thousandflowers/canary/internal/config"
)

// version is stamped at build time by the release tooling. A binary built by
// hand says so rather than claiming a release number it does not have.
var version = "dev"

const usage = `canary — a pixel-art bird that wilts while you grind.

  canary                      draw the bird, with its score
  canary score                print the fatigue score, 0-100
  canary reset                start the session over, bird young again
  canary prompt               the per-prompt draw (your shell hook calls this)
  canary record -- CMD        accrue one command (your shell hook calls this)
  canary statusline           the Claude Code status row (reads its JSON on stdin)
  canary preview --state worn --note falling
  canary preview --state fresh --phrase "some candidate line"
  canary init zsh|bash|fish   print the shell hook to source
  canary settings install     wire the status row into Claude Code's settings.json
  canary settings remove      unwire it again
  canary version

Knobs are environment variables; see the README. CANARY_DISABLED=1 silences
the bird everywhere except an explicit ` + "`canary`" + ` or ` + "`canary score`" + `.`

func main() {
	cfg := config.FromEnv()

	cmd := "status"
	args := os.Args[1:]
	if len(args) > 0 {
		cmd, args = args[0], args[1:]
	}

	switch cmd {
	case "statusline":
		os.Exit(runStatusline(cfg))
	case "prompt":
		os.Exit(runPrompt(cfg))
	case "status":
		os.Exit(runStatus(cfg))
	case "score":
		os.Exit(runScore(cfg))
	case "record":
		os.Exit(runRecord(cfg, args))
	case "reset":
		os.Exit(runReset(cfg))
	case "preview":
		os.Exit(runPreview(cfg, args))
	case "init":
		os.Exit(runInit(args))
	case "settings":
		os.Exit(runSettings(cfg, args))
	case "version", "--version", "-v":
		fmt.Println("canary " + version)
	case "help", "-h", "--help":
		fmt.Println(usage)
	default:
		fmt.Fprintf(os.Stderr, "canary: unknown command: %s\n\n%s\n", cmd, usage)
		os.Exit(2)
	}
}

// fail reports an error the person can act on and returns the exit code. The
// bird never crashes a prompt or a status row over a write it could not make:
// those paths swallow errors, and only the explicit commands complain.
func fail(err error) int {
	fmt.Fprintf(os.Stderr, "canary: %v\n", err)
	return 1
}
