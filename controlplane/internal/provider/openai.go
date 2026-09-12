package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/tryy3/agent-fabric/internal/runtime"
)

const maxErrorBody = 4 << 10

type OpenAI struct {
	baseURL    string
	apiKey     string
	model      string
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

func NewOpenAI(baseURL, apiKey, model string, httpClient *http.Client) *OpenAI {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &OpenAI{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		model:      model,
		httpClient: httpClient,
	}
}

func (o *OpenAI) StreamChat(ctx context.Context, messages []runtime.Message, onDelta func(string) error) error {
	body, err := json.Marshal(chatRequest{
		Model:    o.model,
		Stream:   true,
		Messages: messages,
	})
	if err != nil {
		return fmt.Errorf("marshal chat request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create chat request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+o.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := o.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("send chat request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		snippet, readErr := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
		if readErr != nil {
			return fmt.Errorf("OpenAI HTTP %s: read error body: %w", resp.Status, readErr)
		}
		return fmt.Errorf("OpenAI HTTP %s: %s", resp.Status, strings.TrimSpace(string(snippet)))
	}

	gotContent := false
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
			return fmt.Errorf("decode chat stream: %w", err)
		}
		if len(chunk.Choices) == 0 || chunk.Choices[0].Delta.Content == "" {
			continue
		}
		gotContent = true
		if err := onDelta(chunk.Choices[0].Delta.Content); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("read chat stream: %w", err)
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if !gotContent {
		return fmt.Errorf("empty assistant response")
	}
	return nil
}
