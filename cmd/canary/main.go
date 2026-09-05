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
	"runtime/debug"
	"strings"

	"github.com/thousandflowers/canary/internal/config"
)

// version is stamped at build time by the release tooling. A binary built by
// hand says so rather than claiming a release number it does not have.
var version = "dev"

// versionString is what `canary version` prints.
//
// goreleaser stamps `version` through ldflags. `go install <pkg>@latest`
// cannot: it compiles from source with no flags of ours, so that binary used to
// call itself "dev" while the module version it was built from sat in its own
// build info. The README offers three install paths and promises the same
// binary from each; a version string that depends on which one you took is the
// one place that promise used to break.
//
// A local `go build` reports "(devel)" and keeps saying "dev", which is true.
func versionString(stamped, module string) string {
	if stamped != "dev" {
		return stamped
	}
	if strings.HasPrefix(module, "v") {
		return strings.TrimPrefix(module, "v")
	}
	return stamped
}

const usage = `canary — a pixel-art bird that wilts while you grind.

  canary                      draw the bird, with its score
  canary score                print the fatigue score, 0-100
  canary reset                start the session over, bird young again
  canary chrono               what the bird has learned about your body clock
  canary chrono --bootstrap   seed that from macOS's own screen-time history
  canary prompt               the per-prompt draw (your shell hook calls this)
  canary record -- CMD        accrue one command (your shell hook calls this)
  canary statusline           the Claude Code status row (reads its JSON on stdin)
  canary demo [--state worn]  watch the bird wilt, in place, with no shell in it
  canary preview --state worn --note falling
  canary preview --state fresh --phrase "some candidate line"
  canary lint [corpus-dir]    check phrases against VOICE.md's rules
  canary init zsh|bash|fish   print the shell hook to source
  canary settings install     wire the status row into Claude Code's settings.json
  canary settings remove      unwire it again
  canary version

Knobs are environment variables; see the README. CANARY_DISABLED=1 silences
the bird everywhere except an explicit ` + "`canary`" + ` or ` + "`canary score`" + `.`

// exit is os.Exit, indirected so a test can watch what main does with the code
// run gives it. main is the only thing in this package that cannot be called
// twice, and that is the entire reason it does nothing but this.
var exit = os.Exit

func main() { exit(run(os.Args[1:])) }

// run dispatches one invocation and returns its exit code. Every subcommand
// returns rather than exits, so the whole binary is reachable from a test in
// this package — which is where the interesting cases are: a missing file, a
// settings.json somebody hand-edited, a corpus with a bad line in it.
func run(args []string) int {
	cfg := config.FromEnv()

	cmd := "status"
	if len(args) > 0 {
		cmd, args = args[0], args[1:]
	}

	switch cmd {
	case "statusline":
		return runStatusline(cfg)
	case "prompt":
		return runPrompt(cfg)
	case "status":
		return runStatus(cfg)
	case "score":
		return runScore(cfg)
	case "record":
		return runRecord(cfg, args)
	case "reset":
		return runReset(cfg)
	case "chrono":
		return runChrono(cfg, args)
	case "preview":
		return runPreview(cfg, args)
	case "demo":
		return runDemo(cfg, args)
	case "lint":
		return runLint(cfg, args)
	case "init":
		return runInit(args)
	case "settings":
		return runSettings(cfg, args)
	case "version", "--version", "-v":
		module := ""
		if bi, ok := debug.ReadBuildInfo(); ok {
			module = bi.Main.Version
		}
		fmt.Println("canary " + versionString(version, module))
	case "help", "-h", "--help":
		fmt.Println(usage)
	default:
		fmt.Fprintf(os.Stderr, "canary: unknown command: %s\n\n%s\n", cmd, usage)
		return 2
	}
	return 0
}

// fail reports an error the person can act on and returns the exit code. The
// bird never crashes a prompt or a status row over a write it could not make:
// those paths swallow errors, and only the explicit commands complain.
func fail(err error) int {
	fmt.Fprintf(os.Stderr, "canary: %v\n", err)
	return 1
}
