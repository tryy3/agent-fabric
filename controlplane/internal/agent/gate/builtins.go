package gate

import (
	"fmt"
	"slices"
	"sort"
	"strings"
)

// Tier is one built-in rule tier, described so settings can show it and
// override it: what it catches, the score it gives, whether the scorers are
// consulted afterwards, and for program-list tiers the programs it names.
type Tier struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	// Action is what the tier does on its own: allow, ask or deny. The
	// permission mode still decides from the score for allow and ask.
	Action string `json:"action"`
	// Risk is the tier's base score. Some tiers score a call higher the
	// further it reaches (see Description); an override moves the base only.
	Risk Risk `json:"risk"`
	// Consult reports whether a configured scorer is asked about calls of
	// this tier. Tiers the rules judge reliably are settled without one.
	Consult bool `json:"consult"`
	// Programs lists the program names of a program-list tier, sorted; nil
	// for tiers that are not defined by a program list.
	Programs []string `json:"programs,omitempty"`
	// Locked tiers are structural protections and cannot be overridden.
	Locked bool `json:"locked,omitempty"`
}

// TierOverride changes one built-in tier (settings.permissions.builtins).
type TierOverride struct {
	// Risk replaces the tier's base score.
	Risk Risk `json:"risk,omitempty"`
	// Consult overrides whether scorers are asked about this tier's calls.
	Consult *bool `json:"consult,omitempty"`
	// Add and Remove edit a program-list tier's programs.
	Add    []string `json:"add,omitempty"`
	Remove []string `json:"remove,omitempty"`
}

// Built-in tier IDs.
const (
	TierReadOnly     = "command.read_only"
	TierTooling      = "command.tooling"
	TierDestructive  = "command.destructive"
	TierOpaque       = "command.opaque"
	TierNetwork      = "command.network"
	TierExfiltration = "command.exfiltration"
	TierForbidden    = "command.forbidden"
	TierFileRead     = "file.read"
	TierFileWrite    = "file.write"
	TierFileDelete   = "file.delete"
	TierFileSecret   = "file.secret"
	TierFileRunLater = "file.runs_later"
	TierFileOddName  = "file.odd_name"
	TierOutside      = "path.outside"
	TierProtected    = "protected"
)

// BuiltinTiers describes every built-in tier with its defaults, in the order
// settings show them.
func BuiltinTiers() []Tier {
	list := func(m map[string]bool) []string {
		out := make([]string, 0, len(m))
		for k := range m {
			out = append(out, k)
		}
		sort.Strings(out)
		return out
	}
	return []Tier{
		{ID: TierReadOnly, Title: "Read-only commands", Action: RuleAllow, Risk: riskReadInProject,
			Description: "Listed programs run without asking while their path arguments stay in the project. Reading a secret file scores 3.",
			Programs:    list(readOnlyPrograms)},
		{ID: TierTooling, Title: "Build, test and other commands", Action: RuleAsk, Risk: riskBuildTooling, Consult: true,
			Description: "Every command no other tier claims, including build, test and install tooling. Offers a session grant for the command prefix."},
		{ID: TierDestructive, Title: "Destructive commands", Action: RuleAsk, Risk: riskDestructive, Consult: true,
			Description: "Listed programs, find -delete/-exec and destructive git subcommands. Scores 7 when recursive or forced or aimed outside the project, 8 for the whole project, 9 for process 1.",
			Programs:    list(destructivePrograms)},
		{ID: TierOpaque, Title: "Wrappers and inline code", Action: RuleAsk, Risk: riskOpaque, Consult: true,
			Description: "Listed wrapper programs, shell -c strings, inline interpreter code and git config overrides. Scored higher by what the wrapped command would do.",
			Programs:    list(wrapperPrograms)},
		{ID: TierNetwork, Title: "Network commands", Action: RuleAsk, Risk: riskNetwork,
			Description: "Listed programs that move data to or from other hosts.",
			Programs:    list(networkPrograms)},
		{ID: TierExfiltration, Title: "Exfiltration and download-and-run", Action: RuleAsk, Risk: riskExfil,
			Description: "A network command given a secret, system or home path, or a download piped into a shell."},
		{ID: TierForbidden, Title: "Forbidden programs", Action: RuleDeny, Risk: riskForbidden,
			Description: "Listed privilege and host-control programs are refused. The same name reached by an unusual path asks at 7.",
			Programs:    list(forbiddenPrograms)},
		{ID: TierFileRead, Title: "Reads inside the project", Action: RuleAllow, Risk: riskReadInProject,
			Description: "read_file, list_files and search_text inside the project."},
		{ID: TierFileWrite, Title: "Edits inside the project", Action: RuleAllow, Risk: riskWriteInProject,
			Description: "write_file, append_file, apply_patch, create_directory and move_path inside the project."},
		{ID: TierFileDelete, Title: "Deletes", Action: RuleAsk, Risk: riskDelete, Consult: true,
			Description: "delete_path always asks, even inside the project."},
		{ID: TierFileSecret, Title: "Secret and startup files", Action: RuleAsk, Risk: riskSecretWrite, Consult: true,
			Description: "Writing .env files, keys or credentials. Reading them scores 3; writing shell or git startup files scores 7."},
		{ID: TierFileRunLater, Title: "Files that run later", Action: RuleAllow, Risk: riskRunsLater, Consult: true,
			Description: "Writing CI workflows, git hooks, Makefiles, Dockerfiles and shell scripts. Asks at 7 when the content downloads and runs code or wipes the system; package manifests are checked for the same."},
		{ID: TierFileOddName, Title: "Deceptive file names", Action: RuleAsk, Risk: riskDeceptiveName,
			Description: "File names with control or text-direction characters. A name starting with a dash scores 3."},
		{ID: TierOutside, Title: "Paths outside the project", Action: RuleAsk, Risk: riskPathEscape,
			Description: "Any path outside the sandbox path policy, including ~ and $HOME. Approving elevates the sandbox for that path."},
		{ID: TierProtected, Title: "Protected", Action: RuleDeny, Risk: riskForbidden, Locked: true,
			Description: "Always refused: writes under /etc, /proc, /sys and /dev, the project's .git metadata, recursive wipes of the system, home or parent directory, remote shells, commands outside a Docker environment and working directories outside the project."},
	}
}

// BuiltinTierIDs lists the IDs of BuiltinTiers.
func BuiltinTierIDs() []string {
	var ids []string
	for _, t := range BuiltinTiers() {
		ids = append(ids, t.ID)
	}
	return ids
}

// ValidateBuiltins reports overrides that name an unknown or locked tier, or
// edit the programs of a tier that has none.
func ValidateBuiltins(overrides map[string]TierOverride) error {
	tiers := map[string]Tier{}
	for _, t := range BuiltinTiers() {
		tiers[t.ID] = t
	}
	for id, o := range overrides {
		t, ok := tiers[id]
		switch {
		case !ok:
			return fmt.Errorf("unknown built-in tier %q", id)
		case t.Locked:
			return fmt.Errorf("built-in tier %q cannot be changed", id)
		case o.Risk != 0 && (o.Risk < MinRisk || o.Risk > MaxRisk):
			return fmt.Errorf("built-in tier %q: risk %d outside 1-10", id, o.Risk)
		case t.Programs == nil && (len(o.Add) > 0 || len(o.Remove) > 0):
			return fmt.Errorf("built-in tier %q has no program list", id)
		}
		for _, p := range slices.Concat(o.Add, o.Remove) {
			if strings.TrimSpace(p) == "" || strings.ContainsAny(p, " \t/") {
				return fmt.Errorf("built-in tier %q: %q is not a program name", id, p)
			}
		}
	}
	return nil
}

// ruleset is the built-in tiers with a session's overrides applied.
type ruleset struct {
	forbidden, destructive, network, wrappers, readOnly map[string]bool
	risk                                                map[string]Risk
	base                                                map[string]Risk
	consult                                             map[string]bool
}

var defaultRuleset = newRuleset(nil)

func newRuleset(overrides map[string]TierOverride) *ruleset {
	rs := &ruleset{
		forbidden: forbiddenPrograms, destructive: destructivePrograms, network: networkPrograms,
		wrappers: wrapperPrograms, readOnly: readOnlyPrograms,
		risk: map[string]Risk{}, base: map[string]Risk{}, consult: map[string]bool{},
	}
	lists := map[string]*map[string]bool{
		TierForbidden: &rs.forbidden, TierDestructive: &rs.destructive, TierNetwork: &rs.network,
		TierOpaque: &rs.wrappers, TierReadOnly: &rs.readOnly,
	}
	for _, t := range BuiltinTiers() {
		rs.base[t.ID], rs.consult[t.ID] = t.Risk, t.Consult
		o, ok := overrides[t.ID]
		if !ok || t.Locked {
			continue
		}
		if o.Risk >= MinRisk && o.Risk <= MaxRisk {
			rs.risk[t.ID] = o.Risk
		}
		if o.Consult != nil {
			rs.consult[t.ID] = *o.Consult
		}
		if list := lists[t.ID]; list != nil && (len(o.Add) > 0 || len(o.Remove) > 0) {
			edited := make(map[string]bool, len(*list)+len(o.Add))
			for p := range *list {
				edited[p] = true
			}
			for _, p := range o.Add {
				edited[strings.TrimSpace(p)] = true
			}
			for _, p := range o.Remove {
				delete(edited, strings.TrimSpace(p))
			}
			*list = edited
		}
	}
	return rs
}

// tierOf names the built-in tier that produced a rules decision.
func tierOf(d Decision) string {
	switch d.RuleID {
	case RuleCommandSafe, RuleCommandGranted:
		return TierReadOnly
	case RuleCommandAsk:
		return TierTooling
	case RuleCommandDestructive:
		return TierDestructive
	case RuleCommandOpaque:
		return TierOpaque
	case RuleCommandNetwork:
		return TierNetwork
	case RuleCommandExfil, RuleCommandFetchExec:
		return TierExfiltration
	case RuleCommandForbidden, RuleCommandForbiddenName:
		return TierForbidden
	case "rules.in_policy":
		if d.Access == "" || d.Access == "read" {
			return TierFileRead
		}
		return TierFileWrite
	case RuleDeleteAsk:
		return TierFileDelete
	case RuleSecretPath:
		return TierFileSecret
	case RuleRunsLater:
		return TierFileRunLater
	case RuleOddPath:
		return TierFileOddName
	case "rules.path_ask":
		return TierOutside
	case "rules.ask_user_skip", "rules.unknown_allow":
		return TierFileRead
	}
	return TierProtected
}

// finish applies the tier's risk override and marks the decision Settled when
// the tier does not consult scorers.
func (rs *ruleset) finish(d Decision) Decision {
	tier := tierOf(d)
	if o, ok := rs.risk[tier]; ok && d.Risk == rs.base[tier] && d.Kind != Deny {
		d.Risk = o
	}
	d.Settled = !rs.consult[tier]
	return d
}
