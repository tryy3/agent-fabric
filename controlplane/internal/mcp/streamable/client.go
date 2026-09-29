// Package streamable implements a narrow MCP Streamable HTTP client
// (initialize, tools/list, tools/call) for plane-owned integrations.
package streamable

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
)

// Client is a session-scoped Streamable HTTP MCP client.
type Client struct {
	Endpoint string
	Headers  http.Header
	HTTP     *http.Client

	mu        sync.Mutex
	sessionID string
	nextID    atomic.Int64
	closed    bool
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Tool is a discovered MCP tool.
type Tool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

// Initialize performs MCP initialize + notifications/initialized.
func (c *Client) Initialize(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return fmt.Errorf("mcp client closed")
	}
	result, sessionID, err := c.callLocked(ctx, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    "agent-fabric",
			"version": "0.1.0",
		},
	})
	if err != nil {
		return err
	}
	_ = result
	if sessionID != "" {
		c.sessionID = sessionID
	}
	// Best-effort initialized notification (no response required).
	_, _, _ = c.callLocked(ctx, "notifications/initialized", map[string]any{})
	return nil
}

// ListTools returns tools/list.
func (c *Client) ListTools(ctx context.Context) ([]Tool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	raw, _, err := c.callLocked(ctx, "tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Tools []Tool `json:"tools"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("decode tools/list: %w", err)
	}
	return parsed.Tools, nil
}

// CallTool invokes tools/call and returns the result payload.
func (c *Client) CallTool(ctx context.Context, name string, args map[string]any) (json.RawMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	raw, _, err := c.callLocked(ctx, "tools/call", map[string]any{
		"name":      name,
		"arguments": args,
	})
	if err != nil {
		return nil, err
	}
	return raw, nil
}

// Close marks the client closed (no server teardown API in V1).
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	return nil
}

func (c *Client) callLocked(ctx context.Context, method string, params any) (json.RawMessage, string, error) {
	id := c.nextID.Add(1)
	payload, err := json.Marshal(rpcRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	})
	if err != nil {
		return nil, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	for k, vals := range c.Headers {
		for _, v := range vals {
			req.Header.Add(k, v)
		}
	}
	if c.sessionID != "" {
		req.Header.Set("Mcp-Session-Id", c.sessionID)
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, "", err
	}
	sessionID := resp.Header.Get("Mcp-Session-Id")
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, sessionID, fmt.Errorf("mcp http %d: %s", resp.StatusCode, truncate(string(body)))
	}
	ct := resp.Header.Get("Content-Type")
	rawBody := body
	if strings.Contains(ct, "text/event-stream") {
		rawBody, err = extractSSEData(body)
		if err != nil {
			return nil, sessionID, err
		}
	}
	if method == "notifications/initialized" {
		return json.RawMessage(`{}`), sessionID, nil
	}
	var rpc rpcResponse
	if err := json.Unmarshal(rawBody, &rpc); err != nil {
		return nil, sessionID, fmt.Errorf("decode mcp response: %w", err)
	}
	if rpc.Error != nil {
		return nil, sessionID, fmt.Errorf("mcp error %d: %s", rpc.Error.Code, rpc.Error.Message)
	}
	return rpc.Result, sessionID, nil
}

func extractSSEData(body []byte) ([]byte, error) {
	lines := strings.Split(string(body), "\n")
	var data []string
	for _, line := range lines {
		if strings.HasPrefix(line, "data:") {
			data = append(data, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("sse response missing data")
	}
	return []byte(strings.Join(data, "\n")), nil
}

func truncate(s string) string {
	if len(s) <= 512 {
		return s
	}
	return s[:512] + "…"
}
