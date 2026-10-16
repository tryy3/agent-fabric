package integration

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/tryy3/agent-fabric/internal/httpfetch"
)

// crawl4aiFilter is the Crawl4AI /md content filter: "fit" is its
// readability-style default; "raw" converts the whole DOM.
const crawl4aiFilter = "fit"

// crawl4aiMaxResponse bounds a Crawl4AI /md response. /md echoes the request
// `url` (the whole `raw:` HTML, JSON-escaped, possibly \uXXXX per non-ASCII
// rune) next to the markdown, so it must fit several times the fetch limit.
const crawl4aiMaxResponse = 8 * httpfetch.DefaultMaxBytes

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
