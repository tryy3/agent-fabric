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

type chatRequest struct {
	Model    string            `json:"model"`
	Stream   bool              `json:"stream"`
	Messages []runtime.Message `json:"messages"`
}

type streamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
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

func (o *OpenAI) StreamChat(ctx context.Context, model string, messages []runtime.Message, onDelta func(string) error) error {
	url := o.baseURL + "/chat/completions"
	body, err := json.Marshal(chatRequest{
		Model:    model,
		Stream:   true,
		Messages: messages,
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

	gotContent := false
	deltas := 0
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
		if len(chunk.Choices) == 0 || chunk.Choices[0].Delta.Content == "" {
			continue
		}
		gotContent = true
		deltas++
		if err := onDelta(chunk.Choices[0].Delta.Content); err != nil {
			slog.Error("openai chat onDelta failed", "url", url, "deltas", deltas, "err", err)
			return err
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
	slog.Info("openai chat stream complete",
		"url", url,
		"deltas", deltas,
		"elapsed_ms", time.Since(start).Milliseconds(),
	)
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
