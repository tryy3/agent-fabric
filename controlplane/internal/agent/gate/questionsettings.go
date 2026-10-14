package gate

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"
)

// QuestionSettings edits the gate questions (settings.permissions.questions):
// built-in questions by ID, plus tripwires the user wrote.
type QuestionSettings struct {
	Builtins map[string]QuestionOverride `json:"builtins,omitempty"`
	Custom   []CustomQuestion            `json:"custom,omitempty"`
}

// QuestionOverride changes one built-in question. A policy question ID
// (policy_violation, policy_exception) applies to every family.
type QuestionOverride struct {
	Enabled *bool `json:"enabled,omitempty"`
	// Risk replaces a tripwire's or policy_violation's floor (0 = unchanged).
	Risk Risk `json:"risk,omitempty"`
	// Text replaces the question's wording (empty = unchanged).
	Text string `json:"text,omitempty"`
}

// CustomQuestion is a tripwire the user wrote, asked for the listed tools.
type CustomQuestion struct {
	ID string `json:"id"`
	// Tools are tool names or family names (command, web, file_read, file_write).
	Tools []string `json:"tools"`
	// Kind is tripwire (the default); custom policy questions are not supported.
	Kind QuestionKind `json:"kind,omitempty"`
	Text string       `json:"text"`
	// Risk is the floor a yes sets, 1-10.
	Risk Risk `json:"risk"`
	// Enabled defaults to true.
	Enabled *bool `json:"enabled,omitempty"`
}

// Limits on user-edited questions. The text goes into every scorer request
// for the tool, so it is bounded like the gate instructions are.
const (
	MinQuestionTextChars        = 10
	MaxQuestionTextChars        = 600
	MaxCustomQuestionsPerFamily = 8
)

var questionIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{1,31}$`)

// QuestionLimits describes the validation limits for settings editors.
type QuestionLimits struct {
	IDPattern          string   `json:"idPattern"`
	MinTextChars       int      `json:"minTextChars"`
	MaxTextChars       int      `json:"maxTextChars"`
	MaxCustomPerFamily int      `json:"maxCustomPerFamily"`
	MinRisk            Risk     `json:"minRisk"`
	MaxRisk            Risk     `json:"maxRisk"`
	CustomKinds        []string `json:"customKinds"`
}

// Limits returns the question validation limits.
func Limits() QuestionLimits {
	return QuestionLimits{
		IDPattern: questionIDPattern.String(), MinTextChars: MinQuestionTextChars, MaxTextChars: MaxQuestionTextChars,
		MaxCustomPerFamily: MaxCustomQuestionsPerFamily, MinRisk: MinRisk, MaxRisk: MaxRisk, CustomKinds: []string{string(KindTripwire)},
	}
}

// QuestionInfo is one built-in question with its defaults, as settings list it.
type QuestionInfo struct {
	ID      string       `json:"id"`
	Family  string       `json:"family"`
	Tools   []string     `json:"tools"`
	Kind    QuestionKind `json:"kind"`
	Text    string       `json:"text"`
	Risk    Risk         `json:"risk,omitempty"`
	Enabled bool         `json:"enabled"`
}

// FamilyInfo is one tool family and its tools.
type FamilyInfo struct {
	ID    string   `json:"id"`
	Tools []string `json:"tools"`
}

var familyOrder = []string{familyCommand, familyWeb, familyFileRead, familyFileWrite}

func familyTools(family string) []string {
	var out []string
	for tool, f := range toolFamilies {
		if f == family {
			out = append(out, tool)
		}
	}
	sort.Strings(out)
	return out
}

// QuestionFamilies lists the tool families that have questions.
func QuestionFamilies() []FamilyInfo {
	out := make([]FamilyInfo, 0, len(familyOrder))
	for _, f := range familyOrder {
		out = append(out, FamilyInfo{ID: f, Tools: familyTools(f)})
	}
	return out
}

// BuiltinQuestions lists every built-in question per family with its
// defaults. The policy pair appears once per family with that family's wording.
func BuiltinQuestions() []QuestionInfo {
	var out []QuestionInfo
	for _, f := range familyOrder {
		tools := familyTools(f)
		for _, q := range questionSets[f] {
			out = append(out, QuestionInfo{ID: q.ID, Family: f, Tools: tools, Kind: q.Kind, Text: q.Text, Risk: q.Risk, Enabled: q.Enabled})
		}
	}
	return out
}

func builtinQuestion(id string) (Question, bool) {
	for _, f := range familyOrder {
		for _, q := range questionSets[f] {
			if q.ID == id {
				return q, true
			}
		}
	}
	return Question{}, false
}

// questionText collapses whitespace so user text cannot add lines or
// sections to a scorer prompt.
func questionText(s string) string { return strings.Join(strings.Fields(s), " ") }

func checkText(where, text string) error {
	if n := utf8.RuneCountInString(questionText(text)); n < MinQuestionTextChars || n > MaxQuestionTextChars {
		return fmt.Errorf("%s: text must be %d to %d characters, got %d", where, MinQuestionTextChars, MaxQuestionTextChars, n)
	}
	return nil
}

// customTools resolves a custom question's tools and family names to the
// tool names it applies to and the families those belong to.
func customTools(entries []string) (tools []string, families []string, err error) {
	for _, e := range entries {
		e = strings.TrimSpace(e)
		switch {
		case slices.Contains(familyOrder, e):
			tools = append(tools, familyTools(e)...)
		case toolFamilies[e] != "":
			tools = append(tools, e)
		default:
			return nil, nil, fmt.Errorf("unknown tool or family %q", e)
		}
	}
	sort.Strings(tools)
	tools = slices.Compact(tools)
	for _, t := range tools {
		if f := toolFamilies[t]; !slices.Contains(families, f) {
			families = append(families, f)
		}
	}
	return tools, families, nil
}

// ValidateQuestions reports question settings that name an unknown question,
// break a limit, or change what a question is.
func ValidateQuestions(s QuestionSettings) error {
	for id, o := range s.Builtins {
		q, ok := builtinQuestion(id)
		if !ok {
			return fmt.Errorf("unknown gate question %q", id)
		}
		if o.Risk != 0 {
			if q.ID == QuestionPolicyException {
				return fmt.Errorf("gate question %q has no risk", id)
			}
			if o.Risk < MinRisk || o.Risk > MaxRisk {
				return fmt.Errorf("gate question %q: risk %d outside 1-10", id, o.Risk)
			}
		}
		if o.Text != "" {
			if err := checkText("gate question "+id, o.Text); err != nil {
				return err
			}
		}
	}
	seen := map[string]bool{}
	perFamily := map[string]int{}
	for i, c := range s.Custom {
		where := fmt.Sprintf("custom question %d (%q)", i, c.ID)
		switch {
		case !questionIDPattern.MatchString(c.ID):
			return fmt.Errorf("%s: id must match %s", where, questionIDPattern)
		case seen[c.ID]:
			return fmt.Errorf("%s: id is used twice", where)
		case c.Kind == KindPolicy:
			return fmt.Errorf("%s: custom policy questions are not supported, only tripwires", where)
		case c.Kind != "" && c.Kind != KindTripwire:
			return fmt.Errorf("%s: kind must be tripwire", where)
		case c.Risk < MinRisk || c.Risk > MaxRisk:
			return fmt.Errorf("%s: risk must be 1-10", where)
		case len(c.Tools) == 0:
			return fmt.Errorf("%s: tools is required", where)
		}
		if _, ok := builtinQuestion(c.ID); ok {
			return fmt.Errorf("%s: id is a built-in question", where)
		}
		seen[c.ID] = true
		if err := checkText(where, c.Text); err != nil {
			return err
		}
		_, families, err := customTools(c.Tools)
		if err != nil {
			return fmt.Errorf("%s: %w", where, err)
		}
		for _, f := range families {
			if perFamily[f]++; perFamily[f] > MaxCustomQuestionsPerFamily {
				return fmt.Errorf("%s: at most %d custom questions per family (%s)", where, MaxCustomQuestionsPerFamily, f)
			}
		}
	}
	return nil
}

// QuestionSet is the gate questions of every tool with a session's settings
// applied. The nil set is the built-in one.
type QuestionSet struct {
	byTool map[string][]Question
}

var defaultQuestions = ResolveQuestions(QuestionSettings{})

// ResolveQuestions applies settings to the built-in questions. Settings that
// fail ValidateQuestions are applied as far as they make sense: unknown IDs
// and custom questions without a known tool are skipped, text is capped.
func ResolveQuestions(s QuestionSettings) *QuestionSet {
	qs := &QuestionSet{byTool: map[string][]Question{}}
	for tool, f := range toolFamilies {
		for _, q := range questionSets[f] {
			if o, ok := s.Builtins[q.ID]; ok {
				if o.Enabled != nil {
					q.Enabled = *o.Enabled
				}
				if o.Risk >= MinRisk && o.Risk <= MaxRisk && q.ID != QuestionPolicyException {
					q.Risk = o.Risk
				}
				if t := capText(o.Text); t != "" {
					q.Text, q.UserText = t, true
				}
			}
			qs.byTool[tool] = append(qs.byTool[tool], q)
		}
	}
	for _, c := range s.Custom {
		tools, _, err := customTools(c.Tools)
		text := capText(c.Text)
		if err != nil || text == "" || c.Kind == KindPolicy {
			continue
		}
		q := Question{ID: c.ID, Text: text, Kind: KindTripwire, Risk: ClampRisk(c.Risk), Enabled: c.Enabled == nil || *c.Enabled, UserText: true}
		for _, t := range tools {
			qs.byTool[t] = append(qs.byTool[t], q)
		}
	}
	return qs
}

func capText(s string) string {
	s = questionText(s)
	if utf8.RuneCountInString(s) > MaxQuestionTextChars {
		s = string([]rune(s)[:MaxQuestionTextChars])
	}
	return s
}

func (qs *QuestionSet) orDefault() *QuestionSet {
	if qs == nil {
		return defaultQuestions
	}
	return qs
}

// Tripwires returns the enabled tripwires of a tool, built-in then custom.
func (qs *QuestionSet) Tripwires(tool string) []Question {
	var out []Question
	for _, q := range qs.orDefault().byTool[strings.TrimSpace(tool)] {
		if q.Enabled && q.Kind == KindTripwire {
			out = append(out, q)
		}
	}
	return out
}

// policy returns a tool's enabled policy questions: the violation, then the
// exception when it is enabled. An exception without a violation to cancel
// means nothing, so a disabled violation turns both off.
func (qs *QuestionSet) policy(tool string) []Question {
	var violation, exception *Question
	for _, q := range qs.orDefault().byTool[strings.TrimSpace(tool)] {
		if !q.Enabled || q.Kind != KindPolicy {
			continue
		}
		switch q.ID {
		case QuestionPolicyViolation:
			violation = &q
		case QuestionPolicyException:
			exception = &q
		}
	}
	if violation == nil {
		return nil
	}
	if exception == nil {
		return []Question{*violation}
	}
	return []Question{*violation, *exception}
}
