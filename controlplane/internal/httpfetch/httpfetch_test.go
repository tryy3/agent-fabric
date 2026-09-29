package httpfetch_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tryy3/agent-fabric/internal/httpfetch"
	"github.com/tryy3/agent-fabric/internal/netsafe"
)

func TestGetFollowsSafeRedirectThenBlocksPrivate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://127.0.0.1/secret", http.StatusFound)
	}))
	defer srv.Close()

	_, port, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	c := &httpfetch.Client{
		LookupIP: func(context.Context, string) ([]net.IP, error) {
			return []net.IP{net.ParseIP("93.184.216.34")}, nil
		},
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, net.JoinHostPort("127.0.0.1", port))
		},
	}
	_, err = c.Get(context.Background(), "http://example.test/")
	if err == nil {
		t.Fatal("expected redirect to loopback to be blocked")
	}
	if !strings.Contains(err.Error(), "not allowed") && !strings.Contains(err.Error(), "blocked") {
		t.Fatalf("err = %v", err)
	}
}

func TestAllowedContentType(t *testing.T) {
	if !httpfetch.AllowedContentType("text/html; charset=utf-8") {
		t.Fatal("html")
	}
	if httpfetch.AllowedContentType("application/pdf") {
		t.Fatal("pdf should be rejected")
	}
}

func TestValidateBlocksFileURL(t *testing.T) {
	_, err := netsafe.ValidateURL("file:///etc/passwd")
	if err == nil {
		t.Fatal("expected block")
	}
}
