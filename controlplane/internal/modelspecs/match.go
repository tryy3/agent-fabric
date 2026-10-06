package modelspecs

import (
	"strings"
)

// typeProviderIDs maps built-in connection types to models.dev provider ids.
var typeProviderIDs = map[string][]string{
	"opencode_zen": {"opencode"},
	"opencode_go":  {"opencode-go"},
	"berget_ai":    {"berget"},
}

// ProviderFor finds the specs provider for a connection: by connection type
// first, then by matching baseURL against the provider's api endpoint.
func (s *Service) ProviderFor(connType, baseURL string) (Provider, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, id := range typeProviderIDs[connType] {
		if p, ok := s.providers[id]; ok {
			return p, true
		}
	}
	want := normalizeURL(baseURL)
	if want == "" {
		return Provider{}, false
	}
	for _, p := range s.providers {
		if normalizeURL(p.API) == want {
			return p, true
		}
	}
	return Provider{}, false
}

func normalizeURL(u string) string {
	return strings.ToLower(strings.TrimRight(strings.TrimSpace(u), "/"))
}

// Lookup finds a model by id: exact, then without a "vendor/" prefix, then
// case-insensitively.
func (p Provider) Lookup(id string) (Model, bool) {
	if m, ok := p.Models[id]; ok {
		return m, true
	}
	if i := strings.LastIndex(id, "/"); i >= 0 {
		if m, ok := p.Models[id[i+1:]]; ok {
			return m, true
		}
	}
	for k, m := range p.Models {
		if strings.EqualFold(k, id) {
			return m, true
		}
	}
	return Model{}, false
}

// Ref is the small provider description attached to connections.
type Ref struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Doc     string `json:"doc,omitempty"`
	LogoURL string `json:"logoUrl"`
}

// LogoPath is the plane route that serves a provider logo.
func LogoPath(id string) string { return "/v1/model-specs/providers/" + id + "/logo" }

func (p Provider) Ref() Ref {
	return Ref{ID: p.ID, Name: p.Name, Doc: p.Doc, LogoURL: LogoPath(p.ID)}
}
