package main

import (
	"fmt"
	"os"

	"github.com/thousandflowers/canary/internal/config"
	"github.com/thousandflowers/canary/internal/lint"
	"github.com/thousandflowers/canary/internal/phrase"
)

// runLint checks a phrase corpus against the mechanical half of VOICE.md.
//
// With no argument it lints the corpus compiled into this binary, which is what
// CI does. With a directory it lints that instead, which is what a contributor
// does while editing a line — no rebuild, no Go toolchain.
func runLint(cfg config.Config, args []string) int {
	c := corpus(cfg)
	target := "the corpus in this binary"
	switch len(args) {
	case 0:
	case 1:
		if fi, err := os.Stat(args[0]); err != nil || !fi.IsDir() {
			fmt.Fprintf(os.Stderr, "canary: not a directory: %s\n", args[0])
			return 2
		}
		c = phrase.FromDir(args[0])
		target = args[0]
	default:
		fmt.Fprintln(os.Stderr, "usage: canary lint [corpus-dir]")
		return 2
	}

	findings := lint.Check(c)
	for _, f := range findings {
		fmt.Println(f)
	}
	if len(findings) > 0 {
		fmt.Fprintf(os.Stderr, "\n%d problem(s) in %s\n", len(findings), target)
		return 1
	}
	fmt.Printf("ok — %s passes\n", target)
	return 0
}
