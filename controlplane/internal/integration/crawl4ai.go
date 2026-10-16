package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/httpfetch"
)

// crawl4aiFilter is the Crawl4AI /md content filter: "fit" is its
// readability-style default; "raw" converts the whole DOM.
const crawl4aiFilter = "fit"

var titleRE = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)

// crawl4aiFetch converts plane-fetched bytes through Crawl4AI's POST /md.
// The HTML is sent as a `raw:` URL so Crawl4AI never fetches the page itself
// (the plane fetch stays the SSRF boundary).
func (r *Registry) crawl4aiFetch(ctx context.Context, pin PinnedIntegration, fetched httpfetch.Result) (PageResponse, error) {
	endpoint := strings.TrimRight(pin.Endpoint, "/") + "/md"
	payload, err := json.Marshal(map[string]any{
		"url": "raw:" + string(fetched.Body),
		"f":   crawl4aiFilter,
		"c":   "0",
	})
	if err != nil {
		return PageResponse{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(payload)))
	if err != nil {
		return PageResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.httpClient().Do(req)
	if err != nil {
		return PageResponse{}, unreachableError(pin, err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return PageResponse{}, err
	}
	r.capture(ctx, "http", http.MethodPost, endpoint, resp.StatusCode, truncateCapture(string(payload)), truncateCapture(string(respBody)), map[string]any{
		"integrationId": pin.ID,
		"transport":     "http",
		"kind":          pin.Kind,
		"capability":    "fetch_page",
	})
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return PageResponse{}, fmt.Errorf("%s: POST %s status %d: %s", integrationLabel(pin), endpoint, resp.StatusCode, truncateCapture(string(respBody)))
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

func titleFromHTML(body []byte) string {
	m := titleRE.FindSubmatch(body)
	if m == nil {
		return ""
	}
	return strings.TrimSpace(html.UnescapeString(string(m[1])))
}

// integrationLabel names a pinned integration for model- and user-visible errors.
func integrationLabel(pin PinnedIntegration) string {
	label := pin.Kind
	switch pin.Kind {
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

// unreachableError wraps a transport failure with the integration and its base URL
// so a stopped sidecar or wrong endpoint is obvious from the tool result.
func unreachableError(pin PinnedIntegration, err error) error {
	return fmt.Errorf("%s is unreachable at %s (is the service running and the endpoint correct?): %w", integrationLabel(pin), pin.Endpoint, err)
}
