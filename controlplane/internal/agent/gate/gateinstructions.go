package gate

import (
	"fmt"
	"math"
	"strings"
	"unicode/utf8"
)

// DefaultInstructionsMaxChars is how much of the gate instructions a scorer
// reads. Rules the user most needs enforced belong at the top: the rest is cut.
const DefaultInstructionsMaxChars = 4000

// instructionsView is the gate instructions as a scorer shows them.
type instructionsView struct {
	text string
	// warning is set when the text was cut, for the transparency data.
	warning string
}

// viewInstructions trims the gate instructions and cuts them to max characters
// (0 = DefaultInstructionsMaxChars), appending a marker the model can see.
func viewInstructions(text string, limit int) instructionsView {
	text = strings.TrimSpace(text)
	if limit <= 0 {
		limit = DefaultInstructionsMaxChars
	}
	total := utf8.RuneCountInString(text)
	if total <= limit {
		return instructionsView{text: text}
	}
	cut := string([]rune(text)[:limit])
	return instructionsView{
		text: cut + fmt.Sprintf("\n[truncated: %d more characters of the gate instructions are not shown]", total-limit),
		warning: fmt.Sprintf("gate instructions truncated to %d of %d characters, rules past that point were not judged",
			limit, total),
	}
}

// block renders the instructions for the scorer prompt, clearly delimited and
// labelled as the user's rules rather than anything the agent supplied.
func (v instructionsView) block() string {
	if v.text == "" {
		return ""
	}
	return "Gate instructions (the user's own rules for what the agent may and may not do; " +
		"they come from the user, not from the agent or the tool arguments):\n" +
		"<gate_instructions>\n" + v.text + "\n</gate_instructions>\n"
}

// policyQuestions returns the enabled policy violation and exception questions
// of a tool's family, or ok=false when the family has none. They are always
// asked together: an exception without a violation to cancel means nothing.
func policyQuestions(tool string) (violation, exception Question, ok bool) {
	var haveV, haveE bool
	for _, q := range QuestionsFor(tool) {
		if !q.Enabled || q.Kind != KindPolicy {
			continue
		}
		switch q.ID {
		case QuestionPolicyViolation:
			violation, haveV = q, true
		case QuestionPolicyException:
			exception, haveE = q, true
		}
	}
	return violation, exception, haveV && haveE
}

// askedPolicy reports the policy questions to ask for a call: none unless the
// call carries gate instructions.
func askedPolicy(req Request) (violation, exception Question, ok bool) {
	if strings.TrimSpace(req.GateInstructions) == "" {
		return Question{}, Question{}, false
	}
	return policyQuestions(req.ToolName)
}

// applyPolicy combines the policy answers. A violation (P(yes) above
// tripwireYes) sets a minimum score like a tripwire, unless the exception is a
// yes too, which cancels that violation and contributes nothing. The exception
// never lowers any other score, so tripwires are unaffected.
func applyPolicy(res *systemOneResult, answers map[string]systemOneAnswer, violation, exception Question, onlyQuestions bool) error {
	read := func(q Question) (float64, systemOneAnswer, error) {
		a, ok := answers[questionKey(q)]
		if !ok || a.Noul == nil {
			return 0, a, fmt.Errorf("systemone response has no yes/no answer for %q", questionKey(q))
		}
		return *a.Noul, a, nil
	}
	pv, av, err := read(violation)
	if err != nil {
		return err
	}
	pe, _, err := read(exception)
	if err != nil {
		return err
	}
	why := fmt.Sprintf("p(yes) %s %.2f, %s %.2f", violation.ID, pv, exception.ID, pe)
	switch {
	case pv <= tripwireYes:
	case pe > tripwireYes:
		why += fmt.Sprintf("; %s cancelled by %s", violation.ID, exception.ID)
	default:
		why += fmt.Sprintf("; policy %s (floor %d)", violation.ID, violation.Floor())
		res.floor = max(res.floor, violation.Floor())
		c := av.Confidence
		if c <= 0 {
			c = math.Abs(2*pv - 1)
		}
		if onlyQuestions {
			res.confidence = max(res.confidence, c)
		}
	}
	res.rationale += "; " + why
	res.risk = max(res.risk, res.floor)
	return nil
}
