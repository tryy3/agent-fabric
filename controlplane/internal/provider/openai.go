package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/tryy3/agent-fabric/internal/runtime"
)

const maxErrorBody = 4 << 10

type OpenAI struct {
	baseURL       string
	apiKey        string
	httpClient    *http.Client
	extraHeaders  map[string]string
	samplerExtras bool
	unsloth       bool
}

type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type thinkingParam struct {
	Type string `json:"type"`
}

type chatRequest struct {
	Model             string            `json:"model"`
	Stream            bool              `json:"stream"`
	Messages          []runtime.Message `json:"messages"`
	Tools             []ToolDefinition  `json:"tools,omitempty"`
	StreamOptions     *streamOptions    `json:"stream_options,omitempty"`
	Temperature       *float64          `json:"temperature,omitempty"`
	TopP              *float64          `json:"top_p,omitempty"`
	MaxTokens         *int              `json:"max_tokens,omitempty"`
	ReasoningEffort   *string           `json:"reasoning_effort,omitempty"`
	TopK              *int              `json:"top_k,omitempty"`
	MinP              *float64          `json:"min_p,omitempty"`
	RepetitionPenalty *float64          `json:"repetition_penalty,omitempty"`
	PresencePenalty   *float64          `json:"presence_penalty,omitempty"`
	FrequencyPenalty  *float64          `json:"frequency_penalty,omitempty"`
	EnableThinking    *bool             `json:"enable_thinking,omitempty"`
	ResponseFormat    json.RawMessage   `json:"response_format,omitempty"`
	Thinking          *thinkingParam    `json:"thinking,omitempty"`
}

type streamUsage struct {
	PromptTokens     *int     `json:"prompt_tokens"`
	CompletionTokens *int     `json:"completion_tokens"`
	TotalTokens      *int     `json:"total_tokens"`
	Co2Grams         *float64 `json:"co2_grams"`
	GpuEnergyJoules  *float64 `json:"gpu_energy_joules"`
}

type streamTimings struct {
	PromptMs           *float64 `json:"prompt_ms"`
	PredictedMs        *float64 `json:"predicted_ms"`
	PromptPerSecond    *float64 `json:"prompt_per_second"`
	PredictedPerSecond *float64 `json:"predicted_per_second"`
}

type streamChunk struct {
	Choices []struct {
		Delta struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
			ToolCalls        []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage   json.RawMessage `json:"usage"`
	Timings *streamTimings  `json:"timings"`
}

// knownUsageWireKeys are consumed into typed Usage fields (not copied to Extras).
var knownUsageWireKeys = map[string]struct{}{
	"prompt_tokens":             {},
	"completion_tokens":         {},
	"total_tokens":              {},
	"co2_grams":                 {},
	"co2Grams":                  {},
	"gpu_energy_joules":         {},
	"gpuEnergyJoules":           {},
	"gpu_joules":                {},
	"energy_joules":             {},
	"prompt_tokens_details":     {},
	"completion_tokens_details": {},
}

func parseStreamUsage(raw json.RawMessage) (*streamUsage, map[string]any, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil, nil
	}
	var typed streamUsage
	if err := json.Unmarshal(raw, &typed); err != nil {
		return nil, nil, err
	}
	var bag map[string]any
	if err := json.Unmarshal(raw, &bag); err != nil {
		return &typed, nil, nil
	}
	if typed.Co2Grams == nil {
		typed.Co2Grams = floatFromAnyMap(bag, "co2_grams", "co2Grams")
	}
	if typed.GpuEnergyJoules == nil {
		typed.GpuEnergyJoules = floatFromAnyMap(bag, "gpu_energy_joules", "gpuEnergyJoules", "gpu_joules", "energy_joules")
	}
	extras := make(map[string]any)
	for k, v := range bag {
		if _, known := knownUsageWireKeys[k]; known {
			continue
		}
		extras[snakeToCamelUsageKey(k)] = v
	}
	if len(extras) == 0 {
		extras = nil
	}
	return &typed, extras, nil
}

func floatFromAnyMap(m map[string]any, keys ...string) *float64 {
	for _, k := range keys {
		v, ok := m[k]
		if !ok || v == nil {
			continue
		}
		switch n := v.(type) {
		case float64:
			f := n
			return &f
		case json.Number:
			f, err := n.Float64()
			if err != nil {
				continue
			}
			return &f
		case string:
			f, err := json.Number(n).Float64()
			if err != nil {
				continue
			}
			return &f
		}
	}
	return nil
}

func snakeToCamelUsageKey(key string) string {
	if !strings.Contains(key, "_") {
		return key
	}
	parts := strings.Split(key, "_")
	var b strings.Builder
	for i, p := range parts {
		if p == "" {
			continue
		}
		if i == 0 {
			b.WriteString(p)
			continue
		}
		b.WriteString(strings.ToUpper(p[:1]))
		if len(p) > 1 {
			b.WriteString(p[1:])
		}
	}
	return b.String()
}

func NewOpenAI(baseURL, apiKey string, httpClient *http.Client) *OpenAI {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &OpenAI{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		httpClient: httpClient,
	}
}

// WithExtraHeaders returns a shallow copy that sends the given headers on each request.
func (o *OpenAI) WithExtraHeaders(headers map[string]string) *OpenAI {
	if o == nil {
		return nil
	}
	cp := *o
	if len(headers) == 0 {
		cp.extraHeaders = nil
		return &cp
	}
	cp.extraHeaders = make(map[string]string, len(headers))
	for k, v := range headers {
		cp.extraHeaders[k] = v
	}
	return &cp
}

// WithSamplerExtras returns a shallow copy that includes extended sampler request fields.
func (o *OpenAI) WithSamplerExtras() *OpenAI {
	if o == nil {
		return nil
	}
	cp := *o
	cp.samplerExtras = true
	return &cp
}

// WithUnslothExtras returns a shallow copy that includes Unsloth Studio request fields
// (extended samplers plus enable_thinking).
func (o *OpenAI) WithUnslothExtras() *OpenAI {
	if o == nil {
		return nil
	}
	cp := *o
	cp.samplerExtras = true
	cp.unsloth = true
	return &cp
}

func (o *OpenAI) StreamChat(ctx context.Context, model string, messages []runtime.Message, opts StreamChatOptions, onEvent func(StreamEvent) error) error {
	url := o.baseURL + "/chat/completions"
	reqMessages := messages
	if opts.Instructions != "" {
		reqMessages = make([]runtime.Message, 0, len(messages)+1)
		reqMessages = append(reqMessages, runtime.Message{Role: "system", Content: opts.Instructions})
		reqMessages = append(reqMessages, messages...)
	}
	reqBody := chatRequest{
		Model:           model,
		Stream:          true,
		Messages:        reqMessages,
		Tools:           opts.Tools,
		StreamOptions:   &streamOptions{IncludeUsage: true},
		Temperature:     opts.Temperature,
		TopP:            opts.TopP,
		MaxTokens:       opts.MaxTokens,
		ReasoningEffort: opts.ReasoningEffort,
		ResponseFormat:  opts.ResponseFormat,
	}
	if o.samplerExtras || opts.SamplerExtras {
		reqBody.TopK = opts.TopK
		reqBody.MinP = opts.MinP
		reqBody.RepetitionPenalty = opts.RepetitionPenalty
		reqBody.PresencePenalty = opts.PresencePenalty
		reqBody.FrequencyPenalty = opts.FrequencyPenalty
	}
	if o.unsloth || opts.UnslothExtras {
		reqBody.EnableThinking = opts.EnableThinking
	}
	if opts.BergetExtras && opts.ThinkingType != nil && *opts.ThinkingType != "" {
		reqBody.Thinking = &thinkingParam{Type: *opts.ThinkingType}
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal chat request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create chat request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+o.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	for k, v := range o.extraHeaders {
		req.Header.Set(k, v)
	}
	reqHeaders := cloneHeader(req.Header)

	slog.Info("openai chat request",
		"url", url,
		"model", model,
		"messages", len(messages),
		"body_bytes", len(body),
	)
	start := time.Now()

	resp, err := o.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			slog.Warn("openai chat cancelled before response", "url", url, "err", ctx.Err())
			return ctx.Err()
		}
		slog.Error("openai chat transport error", "url", url, "err", err)
		return fmt.Errorf("send chat request: %w", err)
	}
	defer resp.Body.Close()

	slog.Info("openai chat response headers",
		"url", url,
		"status", resp.StatusCode,
		"content_type", resp.Header.Get("Content-Type"),
		"elapsed_ms", time.Since(start).Milliseconds(),
	)

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		snippet, readErr := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
		if readErr != nil {
			return fmt.Errorf("OpenAI HTTP %s: read error body: %w", resp.Status, readErr)
		}
		trimmed := strings.TrimSpace(string(snippet))
		emitHopCapture(opts, HopCapture{
			Method:      http.MethodPost,
			URL:         url,
			StatusCode:  resp.StatusCode,
			ReqHeaders:  reqHeaders,
			RespHeaders: cloneHeader(resp.Header),
			ReqBody:     body,
			RespBody:    snippet,
			Meta:        map[string]any{"model": model, "provider": "openai_compatible", "error": true},
		})
		slog.Error("openai chat http error", "url", url, "status", resp.StatusCode, "body", trimmed)
		return fmt.Errorf("OpenAI HTTP %s: %s", resp.Status, trimmed)
	}

	streamStart := time.Now()
	gotContent := false
	gotToolCalls := false
	deltas := 0
	var ttftMs int64
	gotTTFT := false
	var lastUsage *streamUsage
	var lastUsageRaw json.RawMessage
	var lastUsageExtras map[string]any
	var lastTimings *streamTimings
	var toolCalls []ToolCall
	var assembledContent strings.Builder
	var assembledThought strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(nil, 1<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}

		var chunk streamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			slog.Error("openai chat bad sse json", "url", url, "data_preview", truncate(data, 200), "err", err)
			return fmt.Errorf("decode chat stream: %w", err)
		}
		if len(chunk.Usage) > 0 && string(chunk.Usage) != "null" {
			parsed, extras, err := parseStreamUsage(chunk.Usage)
			if err != nil {
				slog.Error("openai chat bad usage json", "url", url, "err", err)
				return fmt.Errorf("decode chat usage: %w", err)
			}
			lastUsage = parsed
			lastUsageExtras = extras
			lastUsageRaw = append(json.RawMessage(nil), chunk.Usage...)
		}
		if chunk.Timings != nil {
			tm := *chunk.Timings
			lastTimings = &tm
		}

		var thought, content, finish string
		if len(chunk.Choices) > 0 {
			choice := chunk.Choices[0]
			thought = choice.Delta.ReasoningContent
			content = choice.Delta.Content
			for _, delta := range choice.Delta.ToolCalls {
				for len(toolCalls) <= delta.Index {
					toolCalls = append(toolCalls, ToolCall{})
				}
				toolCalls[delta.Index].ID += delta.ID
				toolCalls[delta.Index].Name += delta.Function.Name
				toolCalls[delta.Index].Arguments += delta.Function.Arguments
			}
			if chunk.Choices[0].FinishReason != nil {
				finish = *chunk.Choices[0].FinishReason
			}
		}
		if finish == "tool_calls" {
			completed := append([]ToolCall(nil), toolCalls...)
			gotToolCalls = len(completed) > 0
			if err := onEvent(StreamEvent{Finish: finish, ToolCalls: completed}); err != nil {
				slog.Error("openai chat onEvent failed", "url", url, "deltas", deltas, "err", err)
				return err
			}
			finish = ""
		}
		if thought == "" && content == "" {
			if finish != "" {
				if err := onEvent(StreamEvent{Finish: finish}); err != nil {
					slog.Error("openai chat onEvent failed", "url", url, "deltas", deltas, "err", err)
					return err
				}
			}
			continue
		}
		if !gotTTFT {
			ttftMs = time.Since(streamStart).Milliseconds()
			gotTTFT = true
		}
		if thought != "" {
			assembledThought.WriteString(thought)
			if err := onEvent(StreamEvent{Thought: thought, Finish: finish}); err != nil {
				slog.Error("openai chat onEvent failed", "url", url, "deltas", deltas, "err", err)
				return err
			}
		}
		if content != "" {
			gotContent = true
			deltas++
			assembledContent.WriteString(content)
			if err := onEvent(StreamEvent{Content: content, Finish: finish}); err != nil {
				slog.Error("openai chat onEvent failed", "url", url, "deltas", deltas, "err", err)
				return err
			}
		}
	}
	if err := scanner.Err(); err != nil {
		if ctx.Err() != nil {
			slog.Warn("openai chat cancelled while streaming", "url", url, "deltas", deltas, "err", ctx.Err())
			return ctx.Err()
		}
		slog.Error("openai chat stream read error", "url", url, "deltas", deltas, "err", err)
		return fmt.Errorf("read chat stream: %w", err)
	}
	if ctx.Err() != nil {
		slog.Warn("openai chat cancelled after stream", "url", url, "deltas", deltas, "err", ctx.Err())
		return ctx.Err()
	}
	if !gotContent && !gotToolCalls {
		slog.Error("openai chat empty assistant", "url", url, "model", model, "messages", len(messages))
		return fmt.Errorf("empty assistant response")
	}
	usage := &Usage{
		Deltas:    deltas,
		TTFTMs:    ptrInt64(ttftMs),
		ElapsedMs: ptrInt64(time.Since(streamStart).Milliseconds()),
		Extras:    lastUsageExtras,
	}
	if lastUsage != nil {
		usage.PromptTokens = lastUsage.PromptTokens
		usage.CompletionTokens = lastUsage.CompletionTokens
		usage.TotalTokens = lastUsage.TotalTokens
		usage.Co2Grams = lastUsage.Co2Grams
		usage.GpuEnergyJoules = lastUsage.GpuEnergyJoules
	}
	if lastTimings != nil {
		usage.PromptMs = lastTimings.PromptMs
		usage.PredictedMs = lastTimings.PredictedMs
		usage.PromptPerSecond = lastTimings.PromptPerSecond
		usage.PredictedPerSecond = lastTimings.PredictedPerSecond
	}
	if err := onEvent(StreamEvent{Usage: usage}); err != nil {
		slog.Error("openai chat onEvent failed", "url", url, "deltas", deltas, "err", err)
		return err
	}
	respPayload := map[string]any{
		"content": assembledContent.String(),
		"thought": assembledThought.String(),
	}
	if len(toolCalls) > 0 {
		respPayload["tool_calls"] = toolCalls
	}
	if len(lastUsageRaw) > 0 {
		var usageObj any
		if err := json.Unmarshal(lastUsageRaw, &usageObj); err == nil {
			respPayload["usage"] = usageObj
		}
	}
	if lastTimings != nil {
		respPayload["timings"] = lastTimings
	}
	respBytes, _ := json.Marshal(respPayload)
	emitHopCapture(opts, HopCapture{
		Method:      http.MethodPost,
		URL:         url,
		StatusCode:  resp.StatusCode,
		ReqHeaders:  reqHeaders,
		RespHeaders: cloneHeader(resp.Header),
		ReqBody:     body,
		RespBody:    respBytes,
		Meta:        map[string]any{"model": model, "provider": "openai_compatible", "deltas": deltas},
	})
	slog.Info("openai chat stream complete",
		"url", url,
		"deltas", deltas,
		"elapsed_ms", time.Since(start).Milliseconds(),
	)
	return nil
}

func ptrInt64(v int64) *int64 { return &v }

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
