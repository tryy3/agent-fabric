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

const anthropicDefaultMaxTokens = 16384

// Anthropic streams the Anthropic Messages API (also used by OpenCode /messages).
type Anthropic struct {
	baseURL      string
	apiKey       string
	httpClient   *http.Client
	extraHeaders map[string]string
}

type anthropicRequest struct {
	Model       string             `json:"model"`
	MaxTokens   int                `json:"max_tokens"`
	Stream      bool               `json:"stream"`
	System      string             `json:"system,omitempty"`
	Messages    []anthropicMessage `json:"messages"`
	Tools       []anthropicTool    `json:"tools,omitempty"`
	Temperature *float64           `json:"temperature,omitempty"`
	TopP        *float64           `json:"top_p,omitempty"`
	Thinking    *anthropicThinking `json:"thinking,omitempty"`
}

type anthropicThinking struct {
	Type         string `json:"type"`
	BudgetTokens int    `json:"budget_tokens,omitempty"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type anthropicTextBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type anthropicToolUseBlock struct {
	Type  string          `json:"type"`
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

type anthropicToolResultBlock struct {
	Type      string `json:"type"`
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"`
}

func NewAnthropic(baseURL, apiKey string, httpClient *http.Client) *Anthropic {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Anthropic{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		httpClient: httpClient,
	}
}

// WithExtraHeaders returns a shallow copy that sends the given headers on each request.
func (a *Anthropic) WithExtraHeaders(headers map[string]string) *Anthropic {
	if a == nil {
		return nil
	}
	cp := *a
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

func (a *Anthropic) StreamChat(ctx context.Context, model string, messages []runtime.Message, opts StreamChatOptions, onEvent func(StreamEvent) error) error {
	system, anthMsgs := toAnthropicMessages(messages)
	// Effective instructions lead; any system-role history messages follow in the same field.
	if opts.Instructions != "" {
		if system != "" {
			system = opts.Instructions + "\n\n" + system
		} else {
			system = opts.Instructions
		}
	}
	tools := toAnthropicTools(opts.Tools)
	maxTokens := anthropicDefaultMaxTokens
	if opts.MaxTokens != nil {
		maxTokens = *opts.MaxTokens
	}
	reqBody := anthropicRequest{
		Model:       model,
		MaxTokens:   maxTokens,
		Stream:      true,
		System:      system,
		Messages:    anthMsgs,
		Tools:       tools,
		Temperature: opts.Temperature,
		TopP:        opts.TopP,
		Thinking:    anthropicThinkingFromOpts(opts),
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal anthropic request: %w", err)
	}

	url := a.baseURL + "/messages"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create anthropic request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("anthropic-version", "2023-06-01")
	for k, v := range a.extraHeaders {
		req.Header.Set(k, v)
	}
	reqHeaders := cloneHeader(req.Header)

	slog.Info("anthropic messages request",
		"url", url,
		"model", model,
		"messages", len(anthMsgs),
		"body_bytes", len(body),
	)
	start := time.Now()

	resp, err := a.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("send anthropic request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		snippet, readErr := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
		if readErr != nil {
			return fmt.Errorf("Anthropic HTTP %s: read error body: %w", resp.Status, readErr)
		}
		emitHopCapture(opts, HopCapture{
			Method:      http.MethodPost,
			URL:         url,
			StatusCode:  resp.StatusCode,
			ReqHeaders:  reqHeaders,
			RespHeaders: cloneHeader(resp.Header),
			ReqBody:     body,
			RespBody:    snippet,
			Meta:        map[string]any{"model": model, "provider": "anthropic", "error": true},
		})
		return fmt.Errorf("Anthropic HTTP %s: %s", resp.Status, strings.TrimSpace(string(snippet)))
	}

	streamStart := time.Now()
	gotContent := false
	gotToolCalls := false
	deltas := 0
	var ttftMs int64
	gotTTFT := false
	var toolCalls []ToolCall
	var currentToolIndex = -1
	var inputTokens, outputTokens *int
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

		var envelope struct {
			Type  string `json:"type"`
			Index int    `json:"index"`
			Delta struct {
				Type         string `json:"type"`
				Text         string `json:"text"`
				Thinking     string `json:"thinking"`
				PartialJSON  string `json:"partial_json"`
				StopReason   string `json:"stop_reason"`
				InputTokens  *int   `json:"input_tokens"`
				OutputTokens *int   `json:"output_tokens"`
			} `json:"delta"`
			ContentBlock struct {
				Type string `json:"type"`
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"content_block"`
			Message struct {
				Usage struct {
					InputTokens  *int `json:"input_tokens"`
					OutputTokens *int `json:"output_tokens"`
				} `json:"usage"`
			} `json:"message"`
			Usage struct {
				InputTokens  *int `json:"input_tokens"`
				OutputTokens *int `json:"output_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data), &envelope); err != nil {
			return fmt.Errorf("decode anthropic stream: %w", err)
		}

		switch envelope.Type {
		case "message_start":
			if envelope.Message.Usage.InputTokens != nil {
				inputTokens = envelope.Message.Usage.InputTokens
			}
		case "content_block_start":
			if envelope.ContentBlock.Type == "tool_use" {
				toolCalls = append(toolCalls, ToolCall{
					ID:   envelope.ContentBlock.ID,
					Name: envelope.ContentBlock.Name,
				})
				currentToolIndex = len(toolCalls) - 1
			}
		case "content_block_delta":
			switch envelope.Delta.Type {
			case "text_delta":
				if envelope.Delta.Text == "" {
					continue
				}
				if !gotTTFT {
					ttftMs = time.Since(streamStart).Milliseconds()
					gotTTFT = true
				}
				gotContent = true
				deltas++
				assembledContent.WriteString(envelope.Delta.Text)
				if err := onEvent(StreamEvent{Content: envelope.Delta.Text}); err != nil {
					return err
				}
			case "thinking_delta":
				if envelope.Delta.Thinking == "" {
					continue
				}
				if !gotTTFT {
					ttftMs = time.Since(streamStart).Milliseconds()
					gotTTFT = true
				}
				assembledThought.WriteString(envelope.Delta.Thinking)
				if err := onEvent(StreamEvent{Thought: envelope.Delta.Thinking}); err != nil {
					return err
				}
			case "input_json_delta":
				if currentToolIndex >= 0 && currentToolIndex < len(toolCalls) {
					toolCalls[currentToolIndex].Arguments += envelope.Delta.PartialJSON
				}
			}
		case "message_delta":
			if envelope.Usage.OutputTokens != nil {
				outputTokens = envelope.Usage.OutputTokens
			}
			if envelope.Delta.StopReason == "tool_use" {
				completed := append([]ToolCall(nil), toolCalls...)
				gotToolCalls = len(completed) > 0
				if err := onEvent(StreamEvent{Finish: "tool_calls", ToolCalls: completed}); err != nil {
					return err
				}
			} else if envelope.Delta.StopReason != "" {
				if err := onEvent(StreamEvent{Finish: "stop"}); err != nil {
					return err
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("read anthropic stream: %w", err)
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if !gotContent && !gotToolCalls {
		return fmt.Errorf("empty assistant response")
	}

	usage := &Usage{
		Deltas:           deltas,
		TTFTMs:           ptrInt64(ttftMs),
		ElapsedMs:        ptrInt64(time.Since(streamStart).Milliseconds()),
		PromptTokens:     inputTokens,
		CompletionTokens: outputTokens,
	}
	if inputTokens != nil || outputTokens != nil {
		total := 0
		if inputTokens != nil {
			total += *inputTokens
		}
		if outputTokens != nil {
			total += *outputTokens
		}
		usage.TotalTokens = &total
	}
	if err := onEvent(StreamEvent{Usage: usage}); err != nil {
		return err
	}
	respPayload := map[string]any{
		"content": assembledContent.String(),
		"thought": assembledThought.String(),
	}
	if len(toolCalls) > 0 {
		respPayload["tool_calls"] = toolCalls
	}
	usagePayload := map[string]any{}
	if inputTokens != nil {
		usagePayload["input_tokens"] = *inputTokens
	}
	if outputTokens != nil {
		usagePayload["output_tokens"] = *outputTokens
	}
	if len(usagePayload) > 0 {
		respPayload["usage"] = usagePayload
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
		Meta:        map[string]any{"model": model, "provider": "anthropic", "deltas": deltas},
	})
	slog.Info("anthropic messages stream complete",
		"url", url,
		"deltas", deltas,
		"elapsed_ms", time.Since(start).Milliseconds(),
	)
	return nil
}

func toAnthropicTools(tools []ToolDefinition) []anthropicTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]anthropicTool, 0, len(tools))
	for _, t := range tools {
		schema := t.Function.Parameters
		if len(schema) == 0 {
			schema = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		out = append(out, anthropicTool{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			InputSchema: schema,
		})
	}
	return out
}

func toAnthropicMessages(messages []runtime.Message) (system string, out []anthropicMessage) {
	for _, m := range messages {
		switch m.Role {
		case "system":
			if system != "" {
				system += "\n\n"
			}
			system += m.Content
		case "user":
			out = append(out, anthropicMessage{Role: "user", Content: m.Content})
		case "assistant":
			if len(m.ToolCalls) > 0 {
				blocks := make([]any, 0, 1+len(m.ToolCalls))
				if m.Content != "" {
					blocks = append(blocks, anthropicTextBlock{Type: "text", Text: m.Content})
				}
				for _, tc := range m.ToolCalls {
					input := json.RawMessage(tc.Function.Arguments)
					if !json.Valid(input) {
						input = json.RawMessage(`{}`)
					}
					blocks = append(blocks, anthropicToolUseBlock{
						Type:  "tool_use",
						ID:    tc.ID,
						Name:  tc.Function.Name,
						Input: input,
					})
				}
				out = append(out, anthropicMessage{Role: "assistant", Content: blocks})
			} else {
				out = append(out, anthropicMessage{Role: "assistant", Content: m.Content})
			}
		case "tool":
			block := anthropicToolResultBlock{
				Type:      "tool_result",
				ToolUseID: m.ToolCallID,
				Content:   m.Content,
			}
			// Anthropic requires tool results as user-role content blocks.
			if len(out) > 0 && out[len(out)-1].Role == "user" {
				if blocks, ok := out[len(out)-1].Content.([]any); ok {
					out[len(out)-1].Content = append(blocks, block)
					continue
				}
			}
			out = append(out, anthropicMessage{Role: "user", Content: []any{block}})
		}
	}
	return system, out
}

func anthropicThinkingFromOpts(opts StreamChatOptions) *anthropicThinking {
	if opts.ReasoningEffort == nil || *opts.ReasoningEffort == "" {
		return nil
	}
	budget := 16384
	switch *opts.ReasoningEffort {
	case "low":
		budget = 4096
	case "medium":
		budget = 16384
	case "high":
		budget = 32768
	case "xhigh", "max":
		budget = 64000
	}
	return &anthropicThinking{Type: "enabled", BudgetTokens: budget}
}

var _ ChatStreamer = (*Anthropic)(nil)
