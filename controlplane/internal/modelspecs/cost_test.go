package modelspecs_test

import (
	"math"
	"testing"

	"github.com/tryy3/agent-fabric/internal/modelspecs"
)

func f(v float64) *float64 { return &v }

func TestEstimate(t *testing.T) {
	tests := []struct {
		name    string
		cost    *modelspecs.Cost
		counts  modelspecs.Counts
		wantOK  bool
		want    float64
		partial bool
	}{
		{
			name:   "plain input and output",
			cost:   &modelspecs.Cost{Input: f(3), Output: f(15)},
			counts: modelspecs.Counts{Prompt: 1_000_000, Completion: 100_000},
			wantOK: true, want: 3 + 1.5,
		},
		{
			name:   "cached read is part of prompt and cheaper",
			cost:   &modelspecs.Cost{Input: f(3), Output: f(15), CacheRead: f(0.3)},
			counts: modelspecs.Counts{Prompt: 1_000_000, Cached: 600_000, Completion: 0},
			wantOK: true, want: 0.4*3 + 0.6*0.3,
		},
		{
			name:   "cache write priced separately",
			cost:   &modelspecs.Cost{Input: f(3), Output: f(15), CacheRead: f(0.3), CacheWrite: f(3.75)},
			counts: modelspecs.Counts{Prompt: 1_000_000, Cached: 200_000, CacheWrite: 100_000},
			wantOK: true, want: 0.7*3 + 0.2*0.3 + 0.1*3.75,
		},
		{
			name:   "cache prices fall back to input price",
			cost:   &modelspecs.Cost{Input: f(2), Output: f(8)},
			counts: modelspecs.Counts{Prompt: 1_000_000, Cached: 500_000, CacheWrite: 500_000},
			wantOK: true, want: 2,
		},
		{
			name:   "reasoning at its own price, not double counted",
			cost:   &modelspecs.Cost{Input: f(1), Output: f(10), Reasoning: f(20)},
			counts: modelspecs.Counts{Completion: 1_000_000, Reasoning: 400_000},
			wantOK: true, want: 0.6*10 + 0.4*20,
		},
		{
			name:   "reasoning billed as output without a reasoning price",
			cost:   &modelspecs.Cost{Input: f(1), Output: f(10)},
			counts: modelspecs.Counts{Completion: 1_000_000, Reasoning: 400_000},
			wantOK: true, want: 10,
		},
		{
			name:   "free model is a real zero",
			cost:   &modelspecs.Cost{Input: f(0), Output: f(0)},
			counts: modelspecs.Counts{Prompt: 5000, Completion: 100},
			wantOK: true, want: 0,
		},
		{
			name:   "missing output price makes it partial",
			cost:   &modelspecs.Cost{Input: f(3)},
			counts: modelspecs.Counts{Prompt: 1_000_000, Completion: 500},
			wantOK: true, want: 3, partial: true,
		},
		{
			name:   "unused component with no price is not partial",
			cost:   &modelspecs.Cost{Input: f(3)},
			counts: modelspecs.Counts{Prompt: 1_000_000},
			wantOK: true, want: 3,
		},
		{name: "no prices means no estimate", cost: &modelspecs.Cost{}, counts: modelspecs.Counts{Prompt: 1}},
		{name: "no cost block means no estimate", cost: nil, counts: modelspecs.Counts{Prompt: 1}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b, ok := modelspecs.Estimate(tc.cost, tc.counts)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v", ok)
			}
			if !ok {
				return
			}
			if math.Abs(b.Total-tc.want) > 1e-9 || b.Partial != tc.partial {
				t.Fatalf("total=%v partial=%v, want %v %v", b.Total, b.Partial, tc.want, tc.partial)
			}
		})
	}
}

func TestBreakdownAdd(t *testing.T) {
	a := modelspecs.Breakdown{Input: 1, Total: 1}
	b := modelspecs.Breakdown{Output: 2, Total: 2, Partial: true}
	s := a.Add(b)
	if s.Total != 3 || s.Input != 1 || s.Output != 2 || !s.Partial {
		t.Fatalf("sum = %+v", s)
	}
}
