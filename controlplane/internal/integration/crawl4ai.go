package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/httpfetch"
)

// crawl4aiFilter is the Crawl4AI /md content filter: "fit" is its
// readability-style default; "raw" converts the whole DOM.
const crawl4aiFilter = "fit"

const (
	// converterMaxResponse bounds a get-md /convert response (markdown only).
	converterMaxResponse = 2 << 20
	// crawl4aiMaxResponse bounds a Crawl4AI /md response. /md echoes the request
	// `url` (the whole `raw:` HTML, JSON-escaped, possibly \uXXXX per non-ASCII
	// rune) next to the markdown, so it must fit several times the fetch limit.
	crawl4aiMaxResponse = 8 * httpfetch.DefaultMaxBytes
)

// titleRE matches the first <title> element (not <titlefoo>).
var titleRE = regexp.MustCompile(`(?is)<title(?:\s[^>]*)?>(.*?)</title>`)

// crawl4aiFetch converts plane-fetched bytes through Crawl4AI's POST /md.
// The HTML is sent as a `raw:` URL so Crawl4AI never fetches the page itself
// (the plane fetch stays the SSRF boundary). Plain-text and Markdown pages skip
// conversion: `raw:` input is always parsed as HTML, which would collapse them.
func (r *Registry) crawl4aiFetch(ctx context.Context, pin PinnedIntegration, fetched httpfetch.Result) (PageResponse, error) {
	if !isHTMLContentType(fetched.ContentType) {
		md, truncated := TruncateMarkdown(string(fetched.Body))
		return PageResponse{
			URL:         fetched.FinalURL,
			FinalURL:    fetched.FinalURL,
			ContentType: fetched.ContentType,
			Markdown:    md,
			Truncated:   truncated,
		}, nil
	}
	respBody, err := r.postConverter(ctx, pin, "/md", map[string]any{
		"url": "raw:" + string(fetched.Body),
		"f":   crawl4aiFilter,
		"c":   "0",
	}, crawl4aiMaxResponse)
	if err != nil {
		return PageResponse{}, err
	}
	var parsed struct {
		Markdown string `json:"markdown"`
		Success  *bool  `json:"success"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return PageResponse{}, fmt.Errorf("%s: decode /md response: %w", integrationLabel(pin), err)
	}
	if parsed.Success != nil && !*parsed.Success {
		return PageResponse{}, fmt.Errorf("%s: /md reported success=false", integrationLabel(pin))
	}
	md, truncated := TruncateMarkdown(parsed.Markdown)
	return PageResponse{
		URL:         fetched.FinalURL,
		FinalURL:    fetched.FinalURL,
		Title:       titleFromHTML(fetched.Body),
		ContentType: fetched.ContentType,
		Markdown:    md,
		Truncated:   truncated,
	}, nil
}

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

// integrationLabel names a pinned integration for model- and user-visible errors.
func integrationLabel(pin PinnedIntegration) string {
	label := pin.Kind
	switch pin.Kind {
	case catalog.KindSearXNG:
		label = "SearXNG"
	case catalog.KindGetMD:
		label = "get-md"
	case catalog.KindCrawl4AI:
		label = "Crawl4AI"
	}
	if pin.Name != "" {
		return fmt.Sprintf("%s integration %q", label, pin.Name)
	}
	return label + " integration"
}

// transportError wraps a transport failure with the integration and its base URL
// so a stopped sidecar or wrong endpoint is obvious from the tool result. A
// cancelled turn is returned as-is, and a timeout is not reported as unreachable.
func transportError(ctx context.Context, pin PinnedIntegration, err error) error {
	if ctx.Err() != nil {
		return err
	}
	endpoint := redactEndpoint(pin.Endpoint)
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return fmt.Errorf("%s at %s timed out: %w", integrationLabel(pin), endpoint, err)
	}
	return fmt.Errorf("%s is unreachable at %s (is the service running and the endpoint correct?): %w", integrationLabel(pin), endpoint, err)
}

// redactEndpoint masks any userinfo password in an endpoint before it is shown
// to the model, the user, or the capture store (net/http strips it from its own
// errors; re-adding the raw endpoint would leak it).
func redactEndpoint(endpoint string) string {
	u, err := url.Parse(endpoint)
	if err != nil || u.User == nil {
		return endpoint
	}
	return u.Redacted()
}
