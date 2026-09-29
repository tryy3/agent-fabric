package integration

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/tryy3/agent-fabric/internal/catalog"
)

// ResolveWebPin loads plane defaults + assistant toolBindings and returns pinned configs.
// Disabled capabilities are omitted (nil). Invalid overrides return an error for session/new.
func ResolveWebPin(ctx context.Context, store *catalog.Store, assistantSettings json.RawMessage) (WebPin, error) {
	settings, err := store.GetPlaneSettings(ctx)
	if err != nil {
		return WebPin{}, err
	}
	bindings, err := catalog.DecodeToolBindings(assistantSettings)
	if err != nil {
		return WebPin{}, err
	}
	var planeSearch, planeFetch string
	if settings.WebSearchIntegrationID != nil {
		planeSearch = *settings.WebSearchIntegrationID
	}
	if settings.FetchPageIntegrationID != nil {
		planeFetch = *settings.FetchPageIntegrationID
	}

	searchResolved, err := catalog.ResolveToolBinding(catalog.CapabilityWebSearch, planeSearch, bindings.WebSearch)
	if err != nil {
		return WebPin{}, err
	}
	fetchResolved, err := catalog.ResolveToolBinding(catalog.CapabilityFetchPage, planeFetch, bindings.FetchPage)
	if err != nil {
		return WebPin{}, err
	}

	var pin WebPin
	if !searchResolved.Disabled {
		p, err := loadPinned(ctx, store, searchResolved.IntegrationID, catalog.CapabilityWebSearch)
		if err != nil {
			return WebPin{}, err
		}
		pin.WebSearch = &p
	}
	if !fetchResolved.Disabled {
		p, err := loadPinned(ctx, store, fetchResolved.IntegrationID, catalog.CapabilityFetchPage)
		if err != nil {
			return WebPin{}, err
		}
		pin.FetchPage = &p
	}
	return pin, nil
}

func loadPinned(ctx context.Context, store *catalog.Store, id, capability string) (PinnedIntegration, error) {
	ti, err := store.GetToolIntegration(ctx, id)
	if err != nil {
		return PinnedIntegration{}, fmt.Errorf("%s integration: %w", capability, err)
	}
	if !ti.Enabled {
		return PinnedIntegration{}, fmt.Errorf("%s integration %q is disabled", capability, id)
	}
	ok := false
	for _, c := range ti.Capabilities {
		if c == capability {
			ok = true
			break
		}
	}
	if !ok {
		return PinnedIntegration{}, fmt.Errorf("integration %q does not support %s", id, capability)
	}
	secrets, err := store.GetToolIntegrationSecrets(ctx, id)
	if err != nil {
		return PinnedIntegration{}, err
	}
	return PinnedIntegration{
		ID:       ti.ID,
		Name:     ti.Name,
		Kind:     ti.Kind,
		Endpoint: ti.Endpoint,
		Mode:     ti.Mode,
		Secrets:  secrets,
		Config:   ti.Config,
	}, nil
}
