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

// Responses streams the OpenAI Responses API (also used by OpenCode /responses).
type Responses struct {
	baseURL      string
	apiKey       string
	httpClient   *http.Client
	extraHeaders map[string]string
}

type responsesRequest struct {
	Model  string          `json:"model"`
	Stream bool            `json:"stream"`
	Input  []responsesItem `json:"input"`
	Tools  []responsesTool `json:"tools,omitempty"`
}

type responsesItem struct {
	Type    string             `json:"type,omitempty"`
	Role    string             `json:"role,omitempty"`
	Content []responsesContent `json:"content,omitempty"`
	// Function call / output fields
	CallID    string `json:"call_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
	Output    string `json:"output,omitempty"`
	ID        string `json:"id,omitempty"`
	Status    string `json:"status,omitempty"`
}

type responsesContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type responsesTool struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

func NewResponses(baseURL, apiKey string, httpClient *http.Client) *Responses {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Responses{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		httpClient: httpClient,
	}
}

// WithExtraHeaders returns a shallow copy that sends the given headers on each request.
func (r *Responses) WithExtraHeaders(headers map[string]string) *Responses {
	if r == nil {
		return nil
	}
	cp := *r
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

func (r *Responses) StreamChat(ctx context.Context, model string, messages []runtime.Message, opts StreamChatOptions, onEvent func(StreamEvent) error) error {
	input := toResponsesInput(messages)
	tools := toResponsesTools(opts.Tools)
	body, err := json.Marshal(responsesRequest{
		Model:  model,
		Stream: true,
		Input:  input,
		Tools:  tools,
	})
	if err != nil {
		return fmt.Errorf("marshal responses request: %w", err)
	}

	url := r.baseURL + "/responses"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create responses request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+r.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	for k, v := range r.extraHeaders {
		req.Header.Set(k, v)
	}

	slog.Info("openai responses request",
		"url", url,
		"model", model,
		"input", len(input),
		"body_bytes", len(body),
	)
	start := time.Now()

	resp, err := r.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("send responses request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		snippet, readErr := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
		if readErr != nil {
			return fmt.Errorf("Responses HTTP %s: read error body: %w", resp.Status, readErr)
		}
		return fmt.Errorf("Responses HTTP %s: %s", resp.Status, strings.TrimSpace(string(snippet)))
	}

	streamStart := time.Now()
	gotContent := false
	gotToolCalls := false
	deltas := 0
	var ttftMs int64
	gotTTFT := false
	toolByIndex := map[int]*ToolCall{}
	var ordered []int
	var promptTokens, completionTokens, totalTokens *int

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
			Delta string `json:"delta"`
			Item  struct {
				Type      string `json:"type"`
				ID        string `json:"id"`
				CallID    string `json:"call_id"`
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			} `json:"item"`
			OutputIndex int `json:"output_index"`
			Response    struct {
				Usage *struct {
					InputTokens  *int `json:"input_tokens"`
					OutputTokens *int `json:"output_tokens"`
					TotalTokens  *int `json:"total_tokens"`
				} `json:"usage"`
				Status string `json:"status"`
			} `json:"response"`
		}
		if err := json.Unmarshal([]byte(data), &envelope); err != nil {
			return fmt.Errorf("decode responses stream: %w", err)
		}

		switch envelope.Type {
		case "response.output_text.delta":
			if envelope.Delta == "" {
				continue
			}
			if !gotTTFT {
				ttftMs = time.Since(streamStart).Milliseconds()
				gotTTFT = true
			}
			gotContent = true
			deltas++
			if err := onEvent(StreamEvent{Content: envelope.Delta}); err != nil {
				return err
			}
		case "response.reasoning_summary_text.delta", "response.reasoning_text.delta":
			if envelope.Delta == "" {
				continue
			}
			if !gotTTFT {
				ttftMs = time.Since(streamStart).Milliseconds()
				gotTTFT = true
			}
			if err := onEvent(StreamEvent{Thought: envelope.Delta}); err != nil {
				return err
			}
		case "response.output_item.added":
			if envelope.Item.Type == "function_call" {
				idx := envelope.OutputIndex
				if _, ok := toolByIndex[idx]; !ok {
					ordered = append(ordered, idx)
				}
				id := envelope.Item.CallID
				if id == "" {
					id = envelope.Item.ID
				}
				toolByIndex[idx] = &ToolCall{
					ID:        id,
					Name:      envelope.Item.Name,
					Arguments: envelope.Item.Arguments,
				}
			}
		case "response.function_call_arguments.delta":
			idx := envelope.OutputIndex
			tc, ok := toolByIndex[idx]
			if !ok {
				toolByIndex[idx] = &ToolCall{Arguments: envelope.Delta}
				ordered = append(ordered, idx)
				continue
			}
			tc.Arguments += envelope.Delta
		case "response.function_call_arguments.done":
			// no-op; arguments already accumulated
		case "response.completed":
			if envelope.Response.Usage != nil {
				promptTokens = envelope.Response.Usage.InputTokens
				completionTokens = envelope.Response.Usage.OutputTokens
				totalTokens = envelope.Response.Usage.TotalTokens
			}
			if len(toolByIndex) > 0 {
				completed := make([]ToolCall, 0, len(ordered))
				for _, idx := range ordered {
					if tc := toolByIndex[idx]; tc != nil {
						completed = append(completed, *tc)
					}
				}
				gotToolCalls = len(completed) > 0
				if gotToolCalls {
					if err := onEvent(StreamEvent{Finish: "tool_calls", ToolCalls: completed}); err != nil {
						return err
					}
				} else if err := onEvent(StreamEvent{Finish: "stop"}); err != nil {
					return err
				}
			} else if err := onEvent(StreamEvent{Finish: "stop"}); err != nil {
				return err
			}
		}
	}
	if err := scanner.Err(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("read responses stream: %w", err)
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
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      totalTokens,
	}
	if err := onEvent(StreamEvent{Usage: usage}); err != nil {
		return err
	}
	slog.Info("openai responses stream complete",
		"url", url,
		"deltas", deltas,
		"elapsed_ms", time.Since(start).Milliseconds(),
	)
	return nil
}

func toResponsesTools(tools []ToolDefinition) []responsesTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]responsesTool, 0, len(tools))
	for _, t := range tools {
		out = append(out, responsesTool{
			Type:        "function",
			Name:        t.Function.Name,
			Description: t.Function.Description,
			Parameters:  t.Function.Parameters,
		})
	}
	return out
}

func toResponsesInput(messages []runtime.Message) []responsesItem {
	out := make([]responsesItem, 0, len(messages))
	for _, m := range messages {
		switch m.Role {
		case "system", "user", "assistant":
			if len(m.ToolCalls) > 0 {
				if m.Content != "" {
					out = append(out, responsesItem{
						Type: "message",
						Role: "assistant",
						Content: []responsesContent{{
							Type: "output_text",
							Text: m.Content,
						}},
					})
				}
				for _, tc := range m.ToolCalls {
					out = append(out, responsesItem{
						Type:      "function_call",
						CallID:    tc.ID,
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					})
				}
				continue
			}
			contentType := "input_text"
			if m.Role == "assistant" {
				contentType = "output_text"
			}
			role := m.Role
			if role == "system" {
				role = "developer"
			}
			out = append(out, responsesItem{
				Type: "message",
				Role: role,
				Content: []responsesContent{{
					Type: contentType,
					Text: m.Content,
				}},
			})
		case "tool":
			out = append(out, responsesItem{
				Type:   "function_call_output",
				CallID: m.ToolCallID,
				Output: m.Content,
			})
		}
	}
	return out
}

var _ ChatStreamer = (*Responses)(nil)
