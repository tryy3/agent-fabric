package integration

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"

	"github.com/tryy3/agent-fabric/internal/catalog"
)

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

func truncateCapture(s string) string {
	const max = 64 << 10
	if len(s) <= max {
		return s
	}
	return s[:max] + "…[truncated]"
}
