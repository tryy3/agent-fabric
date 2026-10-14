package catalog

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// MaxGateInstructionsChars bounds stored gate instructions. The gate scorers
// read a shorter prefix (see gate.DefaultInstructionsMaxChars); the larger
// limit leaves room to write context after the rules that matter most.
const MaxGateInstructionsChars = 20000

// ValidateGateInstructions rejects gate instructions that are too long or not
// valid UTF-8.
func ValidateGateInstructions(text string) error {
	if !utf8.ValidString(text) {
		return fmt.Errorf("gateInstructions must be valid UTF-8")
	}
	if n := utf8.RuneCountInString(text); n > MaxGateInstructionsChars {
		return fmt.Errorf("gateInstructions is %d characters, at most %d are allowed", n, MaxGateInstructionsChars)
	}
	return nil
}

// EffectiveGateInstructions returns the gate instructions of an assistant: its
// own when it sets any (an empty string means none), else the plane-wide
// default. The assistant's text replaces the plane's, it is not appended.
func EffectiveGateInstructions(plane string, assistant *string) string {
	if assistant != nil {
		return strings.TrimSpace(*assistant)
	}
	return strings.TrimSpace(plane)
}
