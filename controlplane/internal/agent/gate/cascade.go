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

// cascadeDisagree is how far below the rules the fast scorer must score before
// the deep scorer is asked to settle it.
const cascadeDisagree = 2

// Cascade is a tiered gate: rules, then a fast scorer, then a deep scorer only
// when the first two leave the call unsettled. Unlike Chain, which runs every
// evaluator and keeps the highest score, it trades scorer calls for latency
// and lets the last tier lower a score:
//
//  1. Rules. A Deny, a permission rule's verdict, a tier the rules judge
//     reliably (Settled), a cancel-band score, or a score at or below
//     SkipAtOrBelow is final.
//  2. Fast (a System One model). It can raise the rules' score but not lower it.
//  3. Deep (a chat model, which sees the earlier scores). Asked when Fast
//     failed, was not confident, or scored well below the rules. It may raise
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
	// cost of whatever the rules misjudge as safe.
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
	if d.Kind == Deny || d.Pinned || d.Settled || d.Risk >= minCancelRisk || d.Risk <= c.SkipAtOrBelow {
		return d, nil
	}
	ruleRisk := d.Risk
	// judged is the highest score a tier stood behind: a fail-closed or
	// unconfident fast score stands when nothing better arrives but does not
	// limit how far the deep tier may lower.
	judged := d.Risk

	escalate := c.Fast == nil
	if c.Fast != nil {
		f, err := c.Fast.Evaluate(ctx, req)
		if err != nil {
			return Decision{}, err
		}
		if f.Kind == Deny {
			f.Scores = appendScore(d.Scores, f)
			return f, nil
		}
		d.Scores = appendScore(d.Scores, f)
		minConf := c.MinConfidence
		if minConf <= 0 {
			minConf = DefaultMinConfidence
		}
		switch {
		case f.Risk == 0: // abstained
		case failedClosed(f), f.Confidence < minConf, f.Risk <= ruleRisk-cascadeDisagree:
			escalate = true
		}
		if f.Risk > d.Risk {
			d.Risk, d.Source, d.Rationale, d.Confidence = f.Risk, f.Source, f.Rationale, f.Confidence
			if !failedClosed(f) && f.Confidence >= minConf {
				judged = f.Risk
			}
		}
	}
	if !escalate || c.Deep == nil {
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
	if deep.Risk == 0 || (failedClosed(deep) && deep.Risk <= d.Risk) {
		return d, nil
	}
	risk, rationale := deep.Risk, deep.Rationale
	maxLower := c.maxLower()
	if req.Tainted {
		maxLower = 0
	}
	if floor := judged - maxLower; risk < floor {
		risk = floor
		rationale = fmt.Sprintf("scored %d, limited to %d (at most %d below the earlier %d): %s", deep.Risk, floor, maxLower, judged, deep.Rationale)
	}
	d.Risk, d.Source, d.Rationale, d.Confidence = risk, deep.Source, rationale, deep.Confidence
	return d, nil
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
	if d.Risk <= 0 {
		return scores
	}
	return append(scores[:len(scores):len(scores)], Score{
		Source: d.Source, Risk: d.Risk, RuleID: d.RuleID, Rationale: d.Rationale, Confidence: d.Confidence,
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
		if s.Confidence > 0 {
			parts[i] += fmt.Sprintf(" (conf %.2f)", s.Confidence)
		}
	}
	return strings.Join(parts, " -> ")
}
