package catalog

import (
	"encoding/json"
	"time"
)

// Tool integration kinds.
const (
	KindSearXNG  = "searxng"
	KindLinkup   = "linkup"
	KindGetMD    = "get_md"
	KindCrawl4AI = "crawl4ai"
)

// Tool integration modes.
const (
	ModeBundled  = "bundled"
	ModeExternal = "external"
)

// Tool integration scope (V1 is plane-only).
const ScopePlane = "plane"

// Stable agent-facing capability names.
const (
	CapabilityWebSearch = "web_search"
	CapabilityFetchPage = "fetch_page"
)

// Binding modes for assistant.settings.toolBindings.
const (
	BindingInherit     = "inherit"
	BindingDisabled    = "disabled"
	BindingIntegration = "integration"
)

// Health statuses for tool integrations.
const (
	HealthUnknown = "unknown"
	HealthHealthy = "healthy"
	HealthUnhealthy = "unhealthy"
)

// BundledDefaultEndpoints are internal DNS endpoints used when mode=bundled.
var BundledDefaultEndpoints = map[string]string{
	KindSearXNG:  "http://searxng:8080",
	KindGetMD:    "http://get-md:3000",
	KindCrawl4AI: "http://crawl4ai:11235",
}

// KindCapabilities returns the capabilities a kind implements in V1.
func KindCapabilities(kind string) []string {
	switch kind {
	case KindSearXNG, KindLinkup:
		return []string{CapabilityWebSearch}
	case KindGetMD, KindCrawl4AI:
		return []string{CapabilityFetchPage}
	default:
		return nil
	}
}

// KindSecretKeys returns write-only secret field names for a kind.
func KindSecretKeys(kind string) []string {
	switch kind {
	case KindLinkup:
		return []string{"apiKey"}
	default:
		return nil
	}
}

// IsKnownIntegrationKind reports whether kind is a built-in V1 integration.
func IsKnownIntegrationKind(kind string) bool {
	switch kind {
	case KindSearXNG, KindLinkup, KindGetMD, KindCrawl4AI:
		return true
	default:
		return false
	}
}

// ToolIntegration is the public catalog DTO (secrets never include values).
type ToolIntegration struct {
	ID                string          `json:"id"`
	Name              string          `json:"name"`
	Kind              string          `json:"kind"`
	Enabled           bool            `json:"enabled"`
	Scope             string          `json:"scope"`
	Endpoint          string          `json:"endpoint"`
	Mode              string          `json:"mode"`
	Capabilities      []string        `json:"capabilities"`
	Config            json.RawMessage `json:"config"`
	SecretsConfigured map[string]bool `json:"secretsConfigured"`
	HealthStatus      string          `json:"healthStatus"`
	HealthCheckedAt   *time.Time      `json:"healthCheckedAt,omitempty"`
	CreatedAt         time.Time       `json:"createdAt"`
	UpdatedAt         time.Time       `json:"updatedAt"`
}

// ToolIntegrationSecrets holds decrypted secret values for runtime use only.
type ToolIntegrationSecrets map[string]string

// CapabilityBinding is one assistant.settings.toolBindings entry.
type CapabilityBinding struct {
	Mode          string `json:"mode"`
	IntegrationID string `json:"integrationId,omitempty"`
}

// ToolBindings is assistant.settings.toolBindings.
type ToolBindings struct {
	WebSearch CapabilityBinding `json:"webSearch"`
	FetchPage CapabilityBinding `json:"fetchPage"`
}
