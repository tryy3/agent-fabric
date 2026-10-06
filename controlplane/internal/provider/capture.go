package provider

import "net/http"

func cloneHeader(h http.Header) http.Header {
	if h == nil {
		return nil
	}
	out := make(http.Header, len(h))
	for k, vals := range h {
		out[k] = append([]string(nil), vals...)
	}
	return out
}

func emitHopCapture(opts StreamChatOptions, c HopCapture) {
	if opts.OnCapture == nil {
		return
	}
	opts.OnCapture(c)
}

// usageCaptureMap is the normalized usage of one request, stored in the hop
// capture's meta beside the raw response body it was read from.
func usageCaptureMap(u *Usage) map[string]any {
	if u == nil {
		return nil
	}
	m := map[string]any{"deltas": u.Deltas}
	setInt := func(k string, v *int) {
		if v != nil {
			m[k] = *v
		}
	}
	setFloat := func(k string, v *float64) {
		if v != nil {
			m[k] = *v
		}
	}
	setInt("promptTokens", u.PromptTokens)
	setInt("completionTokens", u.CompletionTokens)
	setInt("totalTokens", u.TotalTokens)
	setInt("cachedTokens", u.CachedTokens)
	setInt("cacheWriteTokens", u.CacheWriteTokens)
	setInt("reasoningTokens", u.ReasoningTokens)
	setFloat("reportedCostUsd", u.ReportedCostUSD)
	setFloat("promptMs", u.PromptMs)
	setFloat("predictedMs", u.PredictedMs)
	setFloat("promptPerSecond", u.PromptPerSecond)
	setFloat("predictedPerSecond", u.PredictedPerSecond)
	setFloat("co2Grams", u.Co2Grams)
	setFloat("gpuEnergyJoules", u.GpuEnergyJoules)
	if u.TTFTMs != nil {
		m["ttftMs"] = *u.TTFTMs
	}
	if u.ElapsedMs != nil {
		m["elapsedMs"] = *u.ElapsedMs
	}
	for k, v := range u.Extras {
		if _, exists := m[k]; !exists {
			m[k] = v
		}
	}
	return m
}
