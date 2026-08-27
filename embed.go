// Package canary embeds the assets the binary ships with.
//
// The corpus lives at the repo root, not under internal/, because
// CONTRIBUTING.md sends contributors to phrases/en/** and a phrase PR should
// not have to know where the Go code keeps its packages.
package canary

import "embed"

// Corpus is the phrase corpus, compiled into the binary. The shell version
// looked for ~/.canary/phrases and then for a phrases/ dir beside the script,
// and shipped mute whenever the packaging forgot to install it. Embedding
// removes that failure mode: the bird cannot lose its voice in transit.
//
// all: is required — without it embed skips files starting with _ or .
//
//go:embed all:phrases
var Corpus embed.FS
