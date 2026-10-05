package gate

import "github.com/tryy3/agent-fabric/internal/sandbox/sandboxcore"

// Risk is a 1–10 score for how dangerous a tool call is. 0 means unscored:
// a Deny with no risk is a structural refusal (bad arguments, unsupported
// environment) that no permission mode overrides.
type Risk = int

// Band names a range of risk scores.
type Band string

const (
	BandSafe     Band = "safe"     // 1–2: reads, in-project edits
	BandLow      Band = "low"      // 3–4: should be fine, e.g. build/test tooling
	BandElevated Band = "elevated" // 5–6: multi-file, opaque, unrequested destructive
	BandHigh     Band = "high"     // 7–8: dangerous but possibly legitimate
	BandCancel   Band = "cancel"   // 9–10: abort without asking
)

const (
	MinRisk = 1
	MaxRisk = 10
)

// BandOf returns the band of a risk score. Unscored (0) reports "".
func BandOf(risk Risk) Band {
	switch {
	case risk <= 0:
		return ""
	case risk <= 2:
		return BandSafe
	case risk <= 4:
		return BandLow
	case risk <= 6:
		return BandElevated
	case risk <= 8:
		return BandHigh
	default:
		return BandCancel
	}
}

// ClampRisk bounds a score to MinRisk..MaxRisk.
func ClampRisk(risk Risk) Risk {
	return min(max(risk, MinRisk), MaxRisk)
}

// Scores assigned by the deterministic rules. Rules cannot see user intent, so
// they never use the low band for destructive work; in a Cascade the deep
// scorer, which does, may lower a score by a bounded amount.
const (
	riskReadInProject   = 1
	riskWriteInProject  = 2
	riskBuildTooling    = 4 // ask-tier run_command: build, test, install
	riskDelete          = 5
	riskPathEscape      = 7 // a path outside the sandbox policy needs elevation
	riskDestructive     = 6
	riskDestructiveWide = 7 // recursive or forced destructive command
	riskOpaque          = 6
	riskForbidden       = 10

	riskRunsLater          = 3 // writing a CI workflow, build file or script
	riskRunsLaterPayload   = 7 // ...whose content downloads and runs code
	riskSecretRead         = 3 // reading .env or a key file inside the project
	riskOddName            = 3 // a file name that misbehaves as an argument ("-rf")
	riskSecretWrite        = 5
	riskNetwork            = 6
	riskDeceptiveName      = 6 // control or bidi characters in a file name
	riskStartupWrite       = 7 // shell startup files run later with the user's privileges
	riskForbiddenName      = 7
	riskDestructiveProject = 8 // recursive or forced change of the whole project
	riskKillInit           = 9
	riskExfil              = 9
	riskFetchExec          = 9
)

// ruleRisk scores a rules decision from its rule ID. Structural denies (bad
// arguments, no workspace, host execution, cwd escape) stay unscored so no
// mode can relax them.
func ruleRisk(d Decision) Risk {
	switch d.RuleID {
	case "rules.in_policy":
		if d.Access == sandboxcore.PathRead {
			return riskReadInProject
		}
		return riskWriteInProject
	case "rules.path_ask":
		return riskPathEscape
	case RuleDeleteAsk:
		return riskDelete
	case "rules.sensitive_deny", "rules.git_protected",
		RuleCommandForbidden, RuleCommandSensitive, RuleCommandCatastrophic, RuleUserDeny:
		return riskForbidden
	case RuleCommandSafe, RuleCommandGranted:
		return riskReadInProject
	case RuleCommandAsk:
		return riskBuildTooling
	case RuleCommandDestructive:
		return riskDestructive
	case RuleCommandOpaque:
		return riskOpaque
	case RuleCommandNetwork:
		return riskNetwork
	case "rules.ask_user_skip", "rules.unknown_allow":
		return riskReadInProject
	}
	return 0
}
