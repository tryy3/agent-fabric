package integration

import (
	"context"

	"github.com/tryy3/agent-fabric/internal/httpfetch"
)

// ConvertForTest exposes convertFromFetch for unit tests with pre-fetched bytes.
func ConvertForTest(r *Registry, pin PinnedIntegration, fetched httpfetch.Result) (PageResponse, error) {
	return r.convertFromFetch(context.Background(), pin, fetched, "/convert")
}
