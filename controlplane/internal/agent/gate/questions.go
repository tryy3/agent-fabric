package gate

import (
	"strings"

	"github.com/tryy3/agent-fabric/internal/sandbox/tools/command"
)

// QuestionKind says how a gate question's answer is used.
type QuestionKind string

const (
	// KindTripwire is a yes/no question about the nature of a call. A yes sets
	// a minimum risk score (the question's Risk) that no later tier may lower.
	KindTripwire QuestionKind = "tripwire"
	// KindPolicy is a yes/no question about the user's gate instructions. No
	// scorer asks it yet.
	KindPolicy QuestionKind = "policy"
)

// DefaultTripwireRisk is the floor of a tripwire that sets no Risk.
const DefaultTripwireRisk Risk = 8

// Question is a yes/no question a scorer answers about one tool call.
type Question struct {
	ID   string
	Text string
	Kind QuestionKind
	// Risk is the minimum score a yes sets (tripwires only; 0 = DefaultTripwireRisk).
	Risk    Risk
	Enabled bool
}

// Floor is the minimum score a yes on this tripwire sets.
func (q Question) Floor() Risk {
	if q.Risk <= 0 {
		return DefaultTripwireRisk
	}
	return ClampRisk(q.Risk)
}

// Tripwire answers: above tripwireYes counts as yes, below tripwireNo as no,
// and anything between is undecided and counts as nothing.
const (
	tripwireYes = 0.7
	tripwireNo  = 0.3
)

const familyCommand = "command"

// toolFamilies maps a tool name to its question-set family. A tool without a
// family has no questions.
var toolFamilies = map[string]string{command.Name: familyCommand}

var questionSets = map[string][]Question{
	familyCommand: {
		{
			ID: "destructive", Kind: KindTripwire, Risk: 8, Enabled: true,
			Text: "Does this command delete, overwrite or irreversibly destroy data, databases, clusters or infrastructure? " +
				"Effects that version control can recover, and removing build artifacts or caches, are not irreversible. " +
				"In a GitOps repository a push can trigger irreversible infrastructure changes, so judge the actual effect.",
		},
		{
			ID: "credentials", Kind: KindTripwire, Risk: 8, Enabled: true,
			Text: "Does this command contain, print or send credentials, secrets, API keys or tokens?",
		},
	},
}

// QuestionsFor returns the built-in question set of a tool's family, or nil
// when the tool has none.
func QuestionsFor(tool string) []Question {
	set := questionSets[toolFamilies[strings.TrimSpace(tool)]]
	return append([]Question(nil), set...)
}

// Tripwires returns the enabled tripwires of a tool's question set.
func Tripwires(tool string) []Question {
	var out []Question
	for _, q := range QuestionsFor(tool) {
		if q.Enabled && q.Kind == KindTripwire {
			out = append(out, q)
		}
	}
	return out
}
