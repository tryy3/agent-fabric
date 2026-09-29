// Package httpfetch provides an SSRF-safe anonymous HTTP GET client.
package httpfetch

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/tryy3/agent-fabric/internal/netsafe"
)

const (
	DefaultTimeout      = 20 * time.Second
	DefaultMaxBytes     = 5 << 20 // 5 MiB
	DefaultMaxRedirects = 5
)

// Result is a successful fetch.
type Result struct {
	FinalURL    string
	ContentType string
	Body        []byte
	StatusCode  int
}

// Client performs SSRF-safe anonymous GETs.
type Client struct {
	Timeout      time.Duration
	MaxBytes     int64
	MaxRedirects int
	UserAgent    string
	// DialContext overrides dialing (tests).
	DialContext func(ctx context.Context, network, addr string) (net.Conn, error)
	// LookupIP overrides DNS (tests).
	LookupIP func(ctx context.Context, host string) ([]net.IP, error)
}

func (c *Client) timeout() time.Duration {
	if c.Timeout > 0 {
		return c.Timeout
	}
	return DefaultTimeout
}

func (c *Client) maxBytes() int64 {
	if c.MaxBytes > 0 {
		return c.MaxBytes
	}
	return DefaultMaxBytes
}

func (c *Client) maxRedirects() int {
	if c.MaxRedirects > 0 {
		return c.MaxRedirects
	}
	return DefaultMaxRedirects
}

func (c *Client) lookup(ctx context.Context, host string) ([]net.IP, error) {
	if c.LookupIP != nil {
		return c.LookupIP(ctx, host)
	}
	return net.DefaultResolver.LookupIP(ctx, "ip", host)
}

// Get fetches url with SSRF checks before connect and after every redirect.
func (c *Client) Get(ctx context.Context, rawURL string) (Result, error) {
	current := rawURL
	var last Result
	for i := 0; i <= c.maxRedirects(); i++ {
		u, err := netsafe.ValidateURL(current)
		if err != nil {
			return Result{}, err
		}
		host := u.Hostname()
		addrs, err := c.lookup(ctx, host)
		if err != nil {
			return Result{}, fmt.Errorf("resolve %q: %w", host, err)
		}
		if err := netsafe.ValidateResolved(host, addrs); err != nil {
			return Result{}, err
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return Result{}, err
		}
		ua := c.UserAgent
		if ua == "" {
			ua = "agent-fabric-fetch/1.0"
		}
		req.Header.Set("User-Agent", ua)
		req.Header.Set("Accept", "text/html,application/xhtml+xml,text/plain,text/markdown,text/x-markdown")

		client := &http.Client{
			Timeout: c.timeout(),
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
			Transport: &http.Transport{
				DialContext: c.dialContext(),
			},
		}
		resp, err := client.Do(req)
		if err != nil {
			return Result{}, err
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, c.maxBytes()+1))
		_ = resp.Body.Close()
		if readErr != nil {
			return Result{}, readErr
		}
		if int64(len(body)) > c.maxBytes() {
			return Result{}, fmt.Errorf("response exceeds %d bytes", c.maxBytes())
		}

		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			loc := resp.Header.Get("Location")
			if loc == "" {
				return Result{}, fmt.Errorf("redirect without Location")
			}
			next, err := u.Parse(loc)
			if err != nil {
				return Result{}, fmt.Errorf("bad redirect: %w", err)
			}
			current = next.String()
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return Result{}, fmt.Errorf("unexpected status %d", resp.StatusCode)
		}
		ct := resp.Header.Get("Content-Type")
		if !AllowedContentType(ct) {
			return Result{}, fmt.Errorf("unsupported content type %q", ct)
		}
		last = Result{
			FinalURL:    u.String(),
			ContentType: ct,
			Body:        body,
			StatusCode:  resp.StatusCode,
		}
		return last, nil
	}
	return Result{}, fmt.Errorf("too many redirects (max %d)", c.maxRedirects())
}

func (c *Client) dialContext() func(ctx context.Context, network, addr string) (net.Conn, error) {
	if c.DialContext != nil {
		return c.DialContext
	}
	return (&net.Dialer{Timeout: 10 * time.Second}).DialContext
}

// AllowedContentType reports whether ct is HTML, XHTML, plain text, or Markdown.
func AllowedContentType(ct string) bool {
	media := strings.ToLower(strings.TrimSpace(strings.Split(ct, ";")[0]))
	switch media {
	case "text/html", "application/xhtml+xml", "text/plain", "text/markdown", "text/x-markdown":
		return true
	case "":
		// Some servers omit Content-Type; treat as HTML for conversion.
		return true
	default:
		return false
	}
}
