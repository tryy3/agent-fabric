package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/tryy3/agent-fabric/internal/catalog"
)

// titleRE matches the first <title> element (not <titlefoo>).
var titleRE = regexp.MustCompile(`(?is)<title(?:\s[^>]*)?>(.*?)</title>`)

// postConverter POSTs a JSON body to a page converter at pin.Endpoint+path,
// captures the hop, and returns the body of a 2xx response.
func (r *Registry) postConverter(ctx context.Context, pin PinnedIntegration, path string, body any, maxResp int64) ([]byte, error) {
	endpoint := strings.TrimRight(pin.Endpoint, "/") + path
	shownEndpoint := redactEndpoint(endpoint)
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false) // page HTML is full of <, >, &: keep it 1 byte each
	if err := enc.Encode(body); err != nil {
		return nil, err
	}
	payload := buf.Bytes()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.httpClient().Do(req)
	if err != nil {
		return nil, transportError(ctx, pin, err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResp+1))
	if err != nil {
		return nil, err
	}
	tooLarge := int64(len(respBody)) > maxResp
	if tooLarge {
		respBody = respBody[:maxResp]
	}
	r.capture(ctx, "http", http.MethodPost, shownEndpoint, resp.StatusCode, truncateCapture(string(payload)), truncateCapture(string(respBody)), map[string]any{
		"integrationId": pin.ID,
		"transport":     "http",
		"kind":          pin.Kind,
		"capability":    catalog.CapabilityFetchPage,
	})
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s: POST %s status %d: %s", integrationLabel(pin), shownEndpoint, resp.StatusCode, truncateCapture(string(respBody)))
	}
	if tooLarge {
		return nil, fmt.Errorf("%s: POST %s response exceeds %d bytes", integrationLabel(pin), shownEndpoint, maxResp)
	}
	return respBody, nil
}

// isHTMLContentType reports whether a fetched page should go through HTML
// conversion. A missing Content-Type is treated as HTML (see httpfetch).
func isHTMLContentType(ct string) bool {
	switch strings.ToLower(strings.TrimSpace(strings.Split(ct, ";")[0])) {
	case "text/plain", "text/markdown", "text/x-markdown":
		return false
	default:
		return true
	}
}

func titleFromHTML(body []byte) string {
	m := titleRE.FindSubmatch(body)
	if m == nil {
		return ""
	}
	return strings.Join(strings.Fields(html.UnescapeString(string(m[1]))), " ")
}
