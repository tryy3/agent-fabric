package gate

import (
	"context"
	"fmt"
	"strings"
)

// DefaultMinConfidence is the fast scorer confidence below which a Cascade
// asks the deep scorer.
const DefaultMinConfidence = 0.6

// DefaultMaxLower is how many points the deep scorer of a Cascade may take
// off the score the earlier tiers reached.
const DefaultMaxLower = 2

// DefaultSkipAtOrBelow settles calls the rules call safe (reads and
// in-project edits) without a scorer, so most calls of a turn cost nothing.
const DefaultSkipAtOrBelow = 2

// cascadeDisagree is how far below the rules the fast scorer must score before
// the deep scorer is asked to settle it.
const cascadeDisagree = 2

// Cascade is a tiered gate: rules, then a fast scorer, then a deep scorer only
// when the first two leave the call unsettled:
//
//  1. Rules. A Deny, a permission rule's verdict, a tier the rules judge
//     reliably (Settled), a cancel-band score, or a score at or below
//     SkipAtOrBelow is final, unless the tier consults its scorers whatever
//     the score (Decision.Consult) and a scorer asks gate questions about the
//     call. A Decision.QuestionsOnly call is final without such a scorer.
//  2. Fast (a System One model). It can raise the rules' score but not lower it.
//     On a call the rules settle at or below SkipAtOrBelow that it sees only
//     because it asks gate questions, only its tripwire floor counts.
//  3. Deep (a chat model, which sees the earlier scores). Asked when Fast
//     failed, was not confident, scored well below the rules, or abstained
//     undecided (Decision.Undecided). It may raise
//     the score freely but lower it by at most MaxLower points, so a call the
//     earlier tiers found dangerous is still asked about; when it fails the
//     highest score stands.
//
// Either scorer may be nil. Decision.Scores lists every tier that scored, in
// order, so a lowered score stays visible.
type Cascade struct {
	Rules Evaluator
	Fast  Evaluator
	Deep  Evaluator
	// MinConfidence overrides DefaultMinConfidence.
	MinConfidence float64
	// SkipAtOrBelow settles calls the rules score this low without consulting
	// a scorer (0 = always consult). It saves a call on every read, at the
	// cost of whatever the rules misjudge as safe. A rules decision marked
	// Consult (web requests, a tier set to consult in settings) is asked about
	// whatever its score when a scorer asks gate questions about it.
	SkipAtOrBelow Risk
	// MaxLower caps how far Deep may lower the score (0 = DefaultMaxLower,
	// negative = never lower). Tool arguments are untrusted, so a model that is
	// talked into "safe" can only move the score this far, and not at all in
	// a tainted session.
	MaxLower int
}

// Evaluate implements Evaluator.
func (c Cascade) Evaluate(ctx context.Context, req Request) (Decision, error) {
	rules := c.Rules
	if rules == nil {
		rules = Rules{}
	}
	d, err := rules.Evaluate(ctx, req)
	if err != nil {
		return Decision{}, err
	}
	d.Scores = appendScore(nil, d)
	asks := c.asksQuestions(req)
	if d.Kind == Deny || d.Pinned || d.Settled || (d.QuestionsOnly && !asks) || d.Risk >= minCancelRisk ||
		(d.Risk <= c.SkipAtOrBelow && !(d.Consult && asks)) {
		return d, nil
	}
	if c.Fast == nil && c.Deep == nil {
		return d, nil
	}
	if req.ReferencedLoader != nil && req.Referenced == nil {
		req.Referenced = req.ReferencedLoader(ctx)
	}
	ruleRisk := d.Risk
	// questionPass is a call the rules settle as safe that a scorer sees only
	// because it asks gate questions about it. Such a scorer's risk answer
	// would turn the safe call into an ask without a tripwire to show for it
	// (a model reads a docs fetch as a 3 or 4), so only its tripwire floor
	// counts.
	questionPass := d.QuestionsOnly || d.Risk <= c.SkipAtOrBelow
	counted := func(x Decision) Risk {
		if questionPass && !failedClosed(x) {
			return x.Floor
		}
		return x.Risk
	}
	// judged is the highest score a tier stood behind: a fail-closed or
	// unconfident fast score stands when nothing better arrives but does not
	// limit how far the deep tier may lower.
	judged := d.Risk
	// floor is the highest minimum score a fired tripwire set. The final score
	// is never below it, whatever MaxLower allows the deep tier to take off.
	var floor Risk

	// consulted reports whether a tier is asked about the call. On a question
	// pass only a scorer that asks gate questions about it is: a score or band
	// scorer would answer nothing that counts, and as the fast tier it would
	// settle the call before a questions deep tier is asked.
	consulted := func(s Evaluator) bool {
		if s == nil {
			return false
		}
		if !questionPass {
			return true
		}
		q, ok := s.(questionAsker)
		return ok && q.asksQuestionsAbout(req)
	}
	fastOn, deepOn := consulted(c.Fast), consulted(c.Deep)

	escalate := !fastOn
	if fastOn {
		f, err := c.Fast.Evaluate(ctx, req)
		if err != nil {
			return Decision{}, err
		}
		if f.Kind == Deny {
			f.Scores = appendScore(d.Scores, f)
			return f, nil
		}
		d.Scores = appendScore(d.Scores, f)
		floor = max(floor, f.Floor)
		d.Floor = floor
		minConf := c.MinConfidence
		if minConf <= 0 {
			minConf = DefaultMinConfidence
		}
		fr := counted(f)
		switch {
		case fr == 0: // abstained; a clean, decided answer set settles the call
			escalate = f.Undecided
		case failedClosed(f), f.Confidence < minConf, fr <= ruleRisk-cascadeDisagree:
			escalate = true
		}
		if fr > d.Risk {
			d.Risk, d.Source, d.Rationale, d.Confidence = fr, f.Source, f.Rationale, f.Confidence
			if !failedClosed(f) && f.Confidence >= minConf {
				judged = fr
			}
		}
		d.Risk = max(d.Risk, floor)
	}
	if !escalate || !deepOn {
		return d, nil
	}

	deepReq := req
	deepReq.Prior = d.Scores
	deep, err := c.Deep.Evaluate(ctx, deepReq)
	if err != nil {
		return Decision{}, err
	}
	if deep.Kind == Deny {
		deep.Scores = appendScore(d.Scores, deep)
		return deep, nil
	}
	d.Scores = appendScore(d.Scores, deep)
	deepRisk := counted(deep)
	if deepRisk == 0 || (failedClosed(deep) && deepRisk <= d.Risk) {
		return d, nil
	}
	floor = max(floor, deep.Floor)
	d.Floor = floor
	risk, rationale := deepRisk, deep.Rationale
	maxLower := c.maxLower()
	if req.Tainted {
		maxLower = 0
	}
	if lowest := judged - maxLower; risk < lowest {
		risk = lowest
		rationale = fmt.Sprintf("scored %d, limited to %d (at most %d below the earlier %d): %s", deepRisk, lowest, maxLower, judged, deep.Rationale)
	}
	if risk < floor {
		rationale = fmt.Sprintf("scored %d, held at the tripwire floor %d: %s", deepRisk, floor, deep.Rationale)
		risk = floor
	}
	d.Risk, d.Source, d.Rationale, d.Confidence = risk, deep.Source, rationale, deep.Confidence
	return d, nil
}

// questionAsker is a scorer that may ask gate questions about a call.
type questionAsker interface {
	asksQuestionsAbout(req Request) bool
}

// asksQuestions reports whether a scorer of the cascade asks at least one gate
// question about req. Only such a scorer earns a call past SkipAtOrBelow: a
// score or band scorer would turn a safe call into an ask without a tripwire
// to show for it.
func (c Cascade) asksQuestions(req Request) bool {
	for _, s := range []Evaluator{c.Fast, c.Deep} {
		if q, ok := s.(questionAsker); ok && q.asksQuestionsAbout(req) {
			return true
		}
	}
	return false
}

func (c Cascade) maxLower() int {
	switch {
	case c.MaxLower < 0:
		return 0
	case c.MaxLower == 0:
		return DefaultMaxLower
	}
	return c.MaxLower
}

// minCancelRisk is the lowest score of the cancel band.
const minCancelRisk = 9

func appendScore(scores []Score, d Decision) []Score {
	// A scorer that abstained on risk but explained itself (its tripwire
	// answers) stays in the trail.
	if d.Risk <= 0 && d.Floor <= 0 && d.Rationale == "" {
		return scores
	}
	return append(scores[:len(scores):len(scores)], Score{
		Source: d.Source, Risk: d.Risk, RuleID: d.RuleID, Rationale: d.Rationale, Confidence: d.Confidence, Floor: d.Floor,
		Undecided: d.Undecided,
	})
}

// failedClosed reports whether a scorer decision is its fail-closed fallback.
func failedClosed(d Decision) bool {
	return strings.HasSuffix(d.RuleID, ".fail_closed")
}

// ScoreTrail renders scores in evaluation order, e.g.
// "rules 6 -> systemone:jev 3 (conf 0.82) -> llm:gemma 3".
func ScoreTrail(scores []Score) string {
	parts := make([]string, len(scores))
	for i, s := range scores {
		parts[i] = fmt.Sprintf("%s %d", s.Source, s.Risk)
		if s.Risk == 0 {
			parts[i] = s.Source + " no risk"
			if s.Undecided {
				parts[i] = s.Source + " undecided"
			}
		}
		if s.Confidence > 0 {
			parts[i] += fmt.Sprintf(" (conf %.2f)", s.Confidence)
		}
		if s.Floor > 0 {
			parts[i] += fmt.Sprintf(" (floor %d)", s.Floor)
		}
	}
	return strings.Join(parts, " -> ")
}
