package agent

import (
	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/modelspecs"
	"github.com/tryy3/agent-fabric/internal/provider"
)

const costCurrency = "USD"

func derefInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

// countsOf converts a usage report into the cost calculator's token counts.
func countsOf(u provider.Usage) modelspecs.Counts {
	return modelspecs.Counts{
		Prompt:     derefInt(u.PromptTokens),
		Completion: derefInt(u.CompletionTokens),
		Cached:     derefInt(u.CachedTokens),
		CacheWrite: derefInt(u.CacheWriteTokens),
		Reasoning:  derefInt(u.ReasoningTokens),
	}
}

// costTracker accumulates the estimated and provider-reported cost of one
// turn across LLM rounds, using the prices pinned at session/new.
type costTracker struct {
	prices      map[string]*modelspecs.Cost
	total       modelspecs.Breakdown
	hasEstimate bool
	reported    float64
	hasReported bool
	rounds      []catalog.MessageRound
}

func newCostTracker(prices map[string]*modelspecs.Cost) *costTracker {
	return &costTracker{prices: prices}
}

// addRound records one round and returns its record. partIndex is where the
// round's first part lands in the message parts; model is what served it.
func (c *costTracker) addRound(model string, u provider.Usage, partIndex int) catalog.MessageRound {
	rec := catalog.MessageRound{
		Round:            len(c.rounds),
		Model:            model,
		PartIndex:        partIndex,
		PromptTokens:     u.PromptTokens,
		CompletionTokens: u.CompletionTokens,
		CachedTokens:     u.CachedTokens,
		CacheWriteTokens: u.CacheWriteTokens,
		ReasoningTokens:  u.ReasoningTokens,
		ReportedCostUSD:  u.ReportedCostUSD,
	}
	if b, ok := modelspecs.Estimate(c.prices[model], countsOf(u)); ok {
		rec.Cost = &catalog.MessageCost{Currency: costCurrency, Estimated: true, Breakdown: b}
		c.total = c.total.Add(b)
		c.hasEstimate = true
	}
	if u.ReportedCostUSD != nil {
		c.reported += *u.ReportedCostUSD
		c.hasReported = true
	}
	c.rounds = append(c.rounds, rec)
	return rec
}

func (c *costTracker) totalCost() *catalog.MessageCost {
	if !c.hasEstimate {
		return nil
	}
	return &catalog.MessageCost{Currency: costCurrency, Estimated: true, Breakdown: c.total}
}

func (c *costTracker) reportedTotal() *float64 {
	if !c.hasReported {
		return nil
	}
	v := c.reported
	return &v
}

// costMeta renders a cost for ACP _meta.
func costMeta(c *catalog.MessageCost) map[string]any {
	m := map[string]any{
		"currency":   c.Currency,
		"estimated":  c.Estimated,
		"total":      c.Total,
		"input":      c.Input,
		"cacheRead":  c.CacheRead,
		"cacheWrite": c.CacheWrite,
		"output":     c.Output,
		"reasoning":  c.Reasoning,
	}
	if c.Partial {
		m["partial"] = true
	}
	return m
}

// roundMeta is the _meta of a mid-turn usage_update: this round's tokens, model
// and cost (the round's cost under "roundCost") plus the running turn totals, so
// clients can show cost between tool calls. Live only; the stored data is the
// message_rounds row.
func (c *costTracker) roundMeta(rec catalog.MessageRound) map[string]any {
	m := map[string]any{"partial": true, "round": rec.Round}
	if rec.Model != "" {
		m["model"] = rec.Model
	}
	setInt := func(k string, v *int) {
		if v != nil {
			m[k] = *v
		}
	}
	setInt("promptTokens", rec.PromptTokens)
	setInt("completionTokens", rec.CompletionTokens)
	setInt("cachedTokens", rec.CachedTokens)
	setInt("cacheWriteTokens", rec.CacheWriteTokens)
	setInt("reasoningTokens", rec.ReasoningTokens)
	if rec.Cost != nil {
		m["roundCost"] = costMeta(rec.Cost)
	}
	if t := c.totalCost(); t != nil {
		m["cost"] = costMeta(t)
	}
	if r := c.reportedTotal(); r != nil {
		m["reportedCostUsd"] = *r
	}
	return m
}
