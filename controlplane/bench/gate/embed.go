// Package gatecases embeds the labelled tool-call dataset the gate benchmark
// (internal/gatebench, cmd/gatebench) scores setups against.
package gatecases

import "embed"

// Cases holds one JSON file of cases per category under cases/.
//
//go:embed cases/*.json
var Cases embed.FS
