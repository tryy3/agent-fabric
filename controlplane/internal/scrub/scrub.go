package scrub

import (
	"context"
	"net/http"
	"strings"
)

// HeaderRedactor redacts secret-bearing HTTP headers before persistence.
type HeaderRedactor interface {
	Redact(h http.Header) http.Header
}

// ContentScrubber scrubs identifying content from request/response bodies.
type ContentScrubber interface {
	Scrub(ctx context.Context, content string) (string, error)
}

// Pipeline applies header redaction then body scrubbing for hop captures.
type Pipeline struct {
	Headers HeaderRedactor
	Body    ContentScrubber
}

// Default builds a pipeline with Go header redaction and the given body scrubber.
func Default(body ContentScrubber) Pipeline {
	if body == nil {
		body = Identity{}
	}
	return Pipeline{Headers: DefaultHeaders{}, Body: body}
}

// RedactHeaders returns a copy of h with secrets replaced.
func (p Pipeline) RedactHeaders(h http.Header) http.Header {
	if p.Headers == nil {
		return DefaultHeaders{}.Redact(h)
	}
	return p.Headers.Redact(h)
}

// ScrubBody runs the content scrubber. On error it returns a fail-closed placeholder.
func (p Pipeline) ScrubBody(ctx context.Context, content string) string {
	if content == "" {
		return ""
	}
	scrubber := p.Body
	if scrubber == nil {
		scrubber = Identity{}
	}
	out, err := scrubber.Scrub(ctx, content)
	if err != nil {
		return "[scrub_failed: body omitted]"
	}
	// Empty output for non-empty input is treated as scrub failure (e.g. CLI no-op).
	if out == "" {
		return "[scrub_failed: body omitted]"
	}
	return out
}

// DefaultHeaders redacts Authorization, cookies, API keys, and secret-shaped values.
type DefaultHeaders struct{}

var secretHeaderNames = map[string]struct{}{
	"Authorization": {},
	"Cookie":        {},
	"Set-Cookie":    {},
	"X-Api-Key":     {},
	"Api-Key":       {},
	"X-Auth-Token":  {},
}

const redacted = "[REDACTED]"

func (DefaultHeaders) Redact(h http.Header) http.Header {
	out := make(http.Header, len(h))
	for k, vals := range h {
		canon := http.CanonicalHeaderKey(k)
		if _, ok := secretHeaderNames[canon]; ok {
			out[canon] = []string{redacted}
			continue
		}
		copied := make([]string, len(vals))
		for i, v := range vals {
			if looksLikeSecret(v) {
				copied[i] = redacted
			} else {
				copied[i] = v
			}
		}
		out[canon] = copied
	}
	return out
}

func looksLikeSecret(v string) bool {
	s := strings.TrimSpace(v)
	if s == "" {
		return false
	}
	lower := strings.ToLower(s)
	if strings.HasPrefix(lower, "bearer ") {
		return true
	}
	if strings.HasPrefix(s, "sk-") || strings.HasPrefix(s, "sk_") {
		return true
	}
	if strings.HasPrefix(s, "api-") && len(s) > 20 {
		return true
	}
	return false
}

// Identity is a no-op ContentScrubber for tests.
type Identity struct{}

func (Identity) Scrub(_ context.Context, content string) (string, error) {
	return content, nil
}

// Fake replaces substrings for hermetic unit tests.
type Fake struct {
	Replacements map[string]string
}

func (f Fake) Scrub(_ context.Context, content string) (string, error) {
	out := content
	for from, to := range f.Replacements {
		out = strings.ReplaceAll(out, from, to)
	}
	return out, nil
}
