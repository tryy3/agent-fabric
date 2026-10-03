package gate

import (
	"fmt"
	"strings"
)

// Mode is a user-facing permission mode: how much of the Gate's risk range the
// agent may run without asking.
type Mode string

const (
	ModeAsk         Mode = "ask"          // Ask for approval
	ModeAutoApprove Mode = "auto_approve" // Approve for me
	ModeAuto        Mode = "auto"         // Run automatically
	ModeFull        Mode = "full"         // Full access
)

// DefaultMode applies when an assistant sets none.
const DefaultMode = ModeAsk

// Modes lists every mode, least to most permissive.
var Modes = []Mode{ModeAsk, ModeAutoApprove, ModeAuto, ModeFull}

// ParseMode validates a mode name. The empty string is DefaultMode.
func ParseMode(s string) (Mode, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return DefaultMode, nil
	}
	for _, m := range Modes {
		if string(m) == s {
			return m, nil
		}
	}
	return "", fmt.Errorf("unknown permission mode %q (want ask, auto_approve, auto or full)", s)
}

// Policy turns a risk score into an outcome: below AskAt runs, from AskAt up
// asks, from CancelAt up is cancelled without asking.
type Policy struct {
	AskAt    Risk
	CancelAt Risk
}

// NeverAsk is an AskAt that no score reaches.
const NeverAsk = MaxRisk + 1

// Policies maps each mode to its thresholds. A later change can load these
// from settings; callers go through PolicyFor.
type Policies map[Mode]Policy

// DefaultPolicies are the built-in thresholds:
//
//	ask           ask from 3 (reads and in-project edits run silently)
//	auto_approve  ask from 5
//	auto          ask from 7
//	full          never ask; cancel only the catastrophic (10)
var DefaultPolicies = Policies{
	ModeAsk:         {AskAt: 3, CancelAt: 9},
	ModeAutoApprove: {AskAt: 5, CancelAt: 9},
	ModeAuto:        {AskAt: 7, CancelAt: 9},
	ModeFull:        {AskAt: NeverAsk, CancelAt: 10},
}

// PolicyFor returns the policy of a mode, falling back to DefaultMode.
func (p Policies) PolicyFor(m Mode) Policy {
	if pol, ok := p[m]; ok {
		return pol
	}
	return p[DefaultMode]
}

// Resolve applies the policy to a chain decision. A Deny is never relaxed and
// an unscored decision (Risk 0) is returned unchanged. Otherwise the score
// decides: cancel, ask, or run. A rule that asked but scores below AskAt is
// marked Overridden so the caller still elevates a path-escaping call
// without prompting.
func (p Policy) Resolve(d Decision) Decision {
	if d.Kind == Deny || d.Risk <= 0 {
		return d
	}
	d.Band = BandOf(d.Risk)
	switch {
	case d.Risk >= p.CancelAt:
		d.Kind = Deny
		d.Reason = fmt.Sprintf("cancelled: risk %d/10 (%s)%s", d.Risk, d.Band, reasonSuffix(d))
	case d.Risk >= p.AskAt:
		if d.Kind != Ask {
			d.Kind = Ask
			d.NoSessionGrant = true
			d.Reason = fmt.Sprintf("risk %d/10 (%s)%s", d.Risk, d.Band, reasonSuffix(d))
		}
	default:
		if d.Kind == Ask {
			d.Kind = Allow
			d.Overridden = true
		}
	}
	return d
}

func reasonSuffix(d Decision) string {
	switch {
	case d.Rationale != "":
		return ": " + d.Rationale
	case d.Reason != "":
		return ": " + d.Reason
	}
	return ""
}
