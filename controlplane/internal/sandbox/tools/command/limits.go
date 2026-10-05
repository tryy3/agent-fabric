package command

import "time"

// Named limits. They are deliberately not part of the tool schema so tuning
// them later does not change what the model sees.
const (
	// MaxOutputBytes caps each of stdout and stderr returned to the model.
	MaxOutputBytes = 64 << 10
	// MaxStdinBytes caps the optional stdin payload.
	MaxStdinBytes = 256 << 10
	// MaxArgs and MaxArgBytes bound the argv array.
	MaxArgs     = 256
	MaxArgBytes = 32 << 10

	DefaultTimeout = 120 * time.Second
	MaxTimeout     = 15 * time.Minute
)

// Stable machine-readable error codes returned in {"error","code"}.
const (
	codeInvalidArgs = "invalid_args"
	codeIO          = "io_error"
)
