package scrub

import "regexp"

var secretShapes = []*regexp.Regexp{
	regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`),
	apiKeyRE,
	regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/=-]{16,}`),
	regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
	regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{30,}`),
	regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{20,}`),
	regexp.MustCompile(`\bxox[baprs]-[A-Za-z0-9-]{10,}`),
	regexp.MustCompile(`\bAIza[0-9A-Za-z_-]{35}\b`),
	regexp.MustCompile(`[A-Za-z][A-Za-z0-9+.-]*://[^\s/:@]+:[^\s/@]{3,}@`),
	// A secret-named variable or key assigned a long literal. The value class
	// leaves out "." and "(" so reading one from the environment is not a hit.
	regexp.MustCompile(`(?i)(api[_-]?key|secret|passw(or)?d|token)[\w.-]*["']?\s*[:=]\s*["']?[A-Za-z0-9/+_=-]{16,}`),
}

// ContainsSecret reports text that holds a credential-shaped value: private
// key blocks, well-known token formats, URLs with embedded passwords, or a
// secret-named assignment of a long literal. It is a conservative detector for
// content that must not leave the plane, not a scrubber.
func ContainsSecret(s string) bool {
	for _, re := range secretShapes {
		if re.MatchString(s) {
			return true
		}
	}
	return false
}
