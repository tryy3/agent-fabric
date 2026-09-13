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
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type chatRequest struct {
	Model         string            `json:"model"`
	Stream        bool              `json:"stream"`
	Messages      []runtime.Message `json:"messages"`
	StreamOptions *streamOptions    `json:"stream_options,omitempty"`
}

type streamUsage struct {
	PromptTokens     *int `json:"prompt_tokens"`
	CompletionTokens *int `json:"completion_tokens"`
	TotalTokens      *int `json:"total_tokens"`
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
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage   *streamUsage   `json:"usage"`
	Timings *streamTimings `json:"timings"`
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

func (o *OpenAI) StreamChat(ctx context.Context, model string, messages []runtime.Message, onEvent func(StreamEvent) error) error {
	url := o.baseURL + "/chat/completions"
	body, err := json.Marshal(chatRequest{
		Model:         model,
		Stream:        true,
		Messages:      messages,
		StreamOptions: &streamOptions{IncludeUsage: true},
	})
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
		slog.Error("openai chat http error", "url", url, "status", resp.StatusCode, "body", trimmed)
		return fmt.Errorf("OpenAI HTTP %s: %s", resp.Status, trimmed)
	}

	streamStart := time.Now()
	gotContent := false
	deltas := 0
	var ttftMs int64
	gotTTFT := false
	var lastUsage *streamUsage
	var lastTimings *streamTimings
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
		if chunk.Usage != nil {
			u := *chunk.Usage
			lastUsage = &u
		}
		if chunk.Timings != nil {
			tm := *chunk.Timings
			lastTimings = &tm
		}

		var thought, content, finish string
		if len(chunk.Choices) > 0 {
			thought = chunk.Choices[0].Delta.ReasoningContent
			content = chunk.Choices[0].Delta.Content
			if chunk.Choices[0].FinishReason != nil {
				finish = *chunk.Choices[0].FinishReason
			}
		}
		if thought == "" && content == "" {
			continue
		}
		if !gotTTFT {
			ttftMs = time.Since(streamStart).Milliseconds()
			gotTTFT = true
		}
		if thought != "" {
			if err := onEvent(StreamEvent{Thought: thought, Finish: finish}); err != nil {
				slog.Error("openai chat onEvent failed", "url", url, "deltas", deltas, "err", err)
				return err
			}
		}
		if content != "" {
			gotContent = true
			deltas++
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
	if !gotContent {
		slog.Error("openai chat empty assistant", "url", url, "model", model, "messages", len(messages))
		return fmt.Errorf("empty assistant response")
	}
	usage := &Usage{
		Deltas:    deltas,
		TTFTMs:    ptrInt64(ttftMs),
		ElapsedMs: ptrInt64(time.Since(streamStart).Milliseconds()),
	}
	if lastUsage != nil {
		usage.PromptTokens = lastUsage.PromptTokens
		usage.CompletionTokens = lastUsage.CompletionTokens
		usage.TotalTokens = lastUsage.TotalTokens
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
