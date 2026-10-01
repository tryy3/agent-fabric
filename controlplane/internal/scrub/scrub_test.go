package scrub_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/tryy3/agent-fabric/internal/scrub"
)

func TestDefaultHeadersRedactsSecrets(t *testing.T) {
	h := http.Header{}
	h.Set("Authorization", "Bearer sk-live-secret")
	h.Set("Cookie", "session=abc")
	h.Set("X-Api-Key", "key-123")
	h.Set("Content-Type", "application/json")
	h.Set("X-Custom", "sk-also-secret")

	out := scrub.DefaultHeaders{}.Redact(h)
	if out.Get("Authorization") != "[REDACTED]" {
		t.Fatalf("Authorization = %q", out.Get("Authorization"))
	}
	if out.Get("Cookie") != "[REDACTED]" {
		t.Fatalf("Cookie = %q", out.Get("Cookie"))
	}
	if out.Get("X-Api-Key") != "[REDACTED]" {
		t.Fatalf("X-Api-Key = %q", out.Get("X-Api-Key"))
	}
	if out.Get("Content-Type") != "application/json" {
		t.Fatalf("Content-Type = %q", out.Get("Content-Type"))
	}
	if out.Get("X-Custom") != "[REDACTED]" {
		t.Fatalf("X-Custom = %q", out.Get("X-Custom"))
	}
	if h.Get("Authorization") != "Bearer sk-live-secret" {
		t.Fatal("original header mutated")
	}
}

func TestPipelineScrubBodyFailClosed(t *testing.T) {
	p := scrub.Pipeline{
		Headers: scrub.DefaultHeaders{},
		Body: scrub.Fake{Replacements: nil},
	}
	// Fake with nil map is fine; use a scrubber that errors
	p.Body = errScrubber{}
	got := p.ScrubBody(context.Background(), `{"key":"sk-secret"}`)
	if got != "[scrub_failed: body omitted]" {
		t.Fatalf("got %q", got)
	}
}

func TestPipelineScrubBodyEmptyOutputFailClosed(t *testing.T) {
	p := scrub.Pipeline{
		Headers: scrub.DefaultHeaders{},
		Body:    emptyScrubber{},
	}
	got := p.ScrubBody(context.Background(), `{"messages":[{"role":"user","content":"hi"}]}`)
	if got != "[scrub_failed: body omitted]" {
		t.Fatalf("got %q", got)
	}
}

type errScrubber struct{}

func (errScrubber) Scrub(context.Context, string) (string, error) {
	return "", context.Canceled
}

type emptyScrubber struct{}

func (emptyScrubber) Scrub(context.Context, string) (string, error) {
	return "", nil
}

func TestFakeScrubber(t *testing.T) {
	f := scrub.Fake{Replacements: map[string]string{"alice@acme.com": "«Email_1»"}}
	got, err := f.Scrub(context.Background(), "mail alice@acme.com")
	if err != nil {
		t.Fatal(err)
	}
	if got != "mail «Email_1»" {
		t.Fatalf("got %q", got)
	}
}

func TestSecretsInText(t *testing.T) {
	in := `OpenAI HTTP 401: {"error":"bad key"} Authorization Bearer sk-live-abcdefghij api sk-test_12345678`
	got := scrub.SecretsInText(in)
	if strings.Contains(got, "sk-live") || strings.Contains(got, "sk-test") {
		t.Fatalf("keys not redacted: %q", got)
	}
	if !strings.Contains(got, "OpenAI HTTP 401") || !strings.Contains(got, "bad key") {
		t.Fatalf("provider body stripped: %q", got)
	}
	if !strings.Contains(got, "[REDACTED]") {
		t.Fatalf("expected redaction marker: %q", got)
	}
}
