package catalog

import (
	"context"
	"fmt"

	"github.com/tryy3/agent-fabric/internal/db"
	"github.com/tryy3/agent-fabric/internal/modelspecs"
)

const roundCostCurrency = "USD"

func int32Ptr(p *int) *int32 {
	if p == nil {
		return nil
	}
	v := int32(*p)
	return &v
}

func intPtr(p *int32) *int {
	if p == nil {
		return nil
	}
	v := int(*p)
	return &v
}

// insertMessageRounds stores the plane-computed rounds of one attempt.
func insertMessageRounds(ctx context.Context, q *db.Queries, messageID string, rounds []MessageRound) error {
	for _, r := range rounds {
		p := db.InsertMessageRoundParams{
			MessageID:        messageID,
			RoundIndex:       int32(r.Round),
			Model:            r.Model,
			PartIndex:        int32(r.PartIndex),
			PromptTokens:     int32Ptr(r.PromptTokens),
			CompletionTokens: int32Ptr(r.CompletionTokens),
			CachedTokens:     int32Ptr(r.CachedTokens),
			CacheWriteTokens: int32Ptr(r.CacheWriteTokens),
			ReasoningTokens:  int32Ptr(r.ReasoningTokens),
			ReportedCostUsd:  r.ReportedCostUSD,
		}
		if c := r.Cost; c != nil {
			p.CostInput = &c.Input
			p.CostCacheRead = &c.CacheRead
			p.CostCacheWrite = &c.CacheWrite
			p.CostOutput = &c.Output
			p.CostReasoning = &c.Reasoning
			p.CostTotal = &c.Total
			p.CostPartial = c.Partial
		}
		if err := q.InsertMessageRound(ctx, p); err != nil {
			return fmt.Errorf("insert message round %d: %w", r.Round, err)
		}
	}
	return nil
}

func messageRoundFromRow(row db.ListMessageRoundsByThreadRow) MessageRound {
	r := MessageRound{
		Round:            int(row.RoundIndex),
		Model:            row.Model,
		PartIndex:        int(row.PartIndex),
		PromptTokens:     intPtr(row.PromptTokens),
		CompletionTokens: intPtr(row.CompletionTokens),
		CachedTokens:     intPtr(row.CachedTokens),
		CacheWriteTokens: intPtr(row.CacheWriteTokens),
		ReasoningTokens:  intPtr(row.ReasoningTokens),
		ReportedCostUSD:  row.ReportedCostUsd,
	}
	if row.CostTotal != nil {
		b := modelspecs.Breakdown{Total: *row.CostTotal, Partial: row.CostPartial}
		if row.CostInput != nil {
			b.Input = *row.CostInput
		}
		if row.CostCacheRead != nil {
			b.CacheRead = *row.CostCacheRead
		}
		if row.CostCacheWrite != nil {
			b.CacheWrite = *row.CostCacheWrite
		}
		if row.CostOutput != nil {
			b.Output = *row.CostOutput
		}
		if row.CostReasoning != nil {
			b.Reasoning = *row.CostReasoning
		}
		r.Cost = &MessageCost{Currency: roundCostCurrency, Estimated: true, Breakdown: b}
	}
	return r
}

// attachRounds loads the thread's rounds onto its messages and sets each
// message's cost to the sum of its rounds.
func (s *Store) attachRounds(ctx context.Context, threadID string, messages []ThreadMessage) error {
	rows, err := s.q.ListMessageRoundsByThread(ctx, threadID)
	if err != nil {
		return fmt.Errorf("list message rounds: %w", err)
	}
	byMessage := make(map[string][]MessageRound)
	for _, row := range rows {
		byMessage[row.MessageID] = append(byMessage[row.MessageID], messageRoundFromRow(row))
	}
	for i := range messages {
		rounds := byMessage[messages[i].ID]
		if len(rounds) == 0 {
			continue
		}
		messages[i].Rounds = rounds
		messages[i].Cost, messages[i].ReportedCostUSD = sumRounds(rounds)
	}
	return nil
}

// sumRounds adds up the priced rounds' costs and the provider-reported costs.
// Partial is set when any round was unpriced or had an unknown price.
func sumRounds(rounds []MessageRound) (*MessageCost, *float64) {
	var total modelspecs.Breakdown
	priced := 0
	var reported float64
	hasReported := false
	for _, r := range rounds {
		if r.Cost != nil {
			total = total.Add(r.Cost.Breakdown)
			priced++
		}
		if r.ReportedCostUSD != nil {
			reported += *r.ReportedCostUSD
			hasReported = true
		}
	}
	var cost *MessageCost
	if priced > 0 {
		total.Partial = total.Partial || priced < len(rounds)
		cost = &MessageCost{Currency: roundCostCurrency, Estimated: true, Breakdown: total}
	}
	var rep *float64
	if hasReported {
		rep = &reported
	}
	return cost, rep
}

// threadTotals sums every round of every attempt in the thread.
func (s *Store) threadTotals(ctx context.Context, threadID string) (ThreadTotals, error) {
	row, err := s.q.SumThreadRounds(ctx, threadID)
	if err != nil {
		return ThreadTotals{}, fmt.Errorf("sum thread rounds: %w", err)
	}
	t := ThreadTotals{
		Turns:            int(row.Turns),
		Requests:         int(row.Requests),
		PromptTokens:     int(row.PromptTokens),
		CompletionTokens: int(row.CompletionTokens),
		CachedTokens:     int(row.CachedTokens),
		CacheWriteTokens: int(row.CacheWriteTokens),
		ReasoningTokens:  int(row.ReasoningTokens),
	}
	if row.PricedRequests > 0 {
		t.Cost = &MessageCost{
			Currency:  roundCostCurrency,
			Estimated: true,
			Breakdown: modelspecs.Breakdown{
				Input:      row.CostInput,
				CacheRead:  row.CostCacheRead,
				CacheWrite: row.CostCacheWrite,
				Output:     row.CostOutput,
				Reasoning:  row.CostReasoning,
				Total:      row.CostTotal,
				Partial:    row.CostPartial || row.PricedRequests < row.Requests,
			},
		}
	}
	if row.ReportedRequests > 0 {
		v := row.ReportedCostUsd
		t.ReportedCostUSD = &v
	}
	return t, nil
}
