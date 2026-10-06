package modelspecs

// Counts are the token counts of one LLM round (or a sum of rounds).
// Prompt is total input including Cached and CacheWrite; Reasoning is the part
// of Completion spent on reasoning.
type Counts struct {
	Prompt, Completion, Cached, CacheWrite, Reasoning int
}

// Breakdown is an estimated cost in USD. Components whose price is unknown
// are 0 and mark the result Partial.
type Breakdown struct {
	Input      float64 `json:"input"`
	CacheRead  float64 `json:"cacheRead"`
	CacheWrite float64 `json:"cacheWrite"`
	Output     float64 `json:"output"`
	Reasoning  float64 `json:"reasoning"`
	Total      float64 `json:"total"`
	Partial    bool    `json:"partial,omitempty"`
}

// Add returns the sum of two breakdowns.
func (b Breakdown) Add(o Breakdown) Breakdown {
	return Breakdown{
		Input:      b.Input + o.Input,
		CacheRead:  b.CacheRead + o.CacheRead,
		CacheWrite: b.CacheWrite + o.CacheWrite,
		Output:     b.Output + o.Output,
		Reasoning:  b.Reasoning + o.Reasoning,
		Total:      b.Total + o.Total,
		Partial:    b.Partial || o.Partial,
	}
}

const perMillion = 1_000_000.0

// Estimate prices the counts with a model's per-million-token prices. ok is
// false when the model has neither an input nor an output price, so there is
// nothing meaningful to show (a price of 0 is real: the model is free).
//
// Cache reads and writes fall back to the input price when no cache price is
// published. Reasoning tokens are billed at the reasoning price when one is
// published, otherwise at the output price; they are never counted twice.
func Estimate(c *Cost, n Counts) (b Breakdown, ok bool) {
	if c == nil || (c.Input == nil && c.Output == nil) {
		return Breakdown{}, false
	}
	price := func(count int, rates ...*float64) float64 {
		if count <= 0 {
			return 0
		}
		for _, r := range rates {
			if r != nil {
				return float64(count) * *r / perMillion
			}
		}
		b.Partial = true
		return 0
	}
	uncached := max(n.Prompt-n.Cached-n.CacheWrite, 0)
	b.Input = price(uncached, c.Input)
	b.CacheRead = price(n.Cached, c.CacheRead, c.Input)
	b.CacheWrite = price(n.CacheWrite, c.CacheWrite, c.Input)

	reasoning := min(max(n.Reasoning, 0), n.Completion)
	if c.Reasoning != nil && reasoning > 0 {
		b.Reasoning = price(reasoning, c.Reasoning)
		b.Output = price(n.Completion-reasoning, c.Output)
	} else {
		b.Output = price(n.Completion, c.Output)
	}
	b.Total = b.Input + b.CacheRead + b.CacheWrite + b.Output + b.Reasoning
	return b, true
}
