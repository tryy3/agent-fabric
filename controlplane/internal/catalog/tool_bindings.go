package catalog

import (
	"encoding/json"
	"fmt"
	"strings"
)

// DecodeToolBindings reads assistant.settings.toolBindings (missing = inherit both).
func DecodeToolBindings(settings json.RawMessage) (ToolBindings, error) {
	out := ToolBindings{
		WebSearch: CapabilityBinding{Mode: BindingInherit},
		FetchPage: CapabilityBinding{Mode: BindingInherit},
	}
	if len(settings) == 0 {
		return out, nil
	}
	var bag map[string]json.RawMessage
	if err := json.Unmarshal(settings, &bag); err != nil {
		return ToolBindings{}, fmt.Errorf("decode settings: %w", err)
	}
	raw, ok := bag["toolBindings"]
	if !ok || isJSONNull(raw) {
		return out, nil
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return ToolBindings{}, fmt.Errorf("decode toolBindings: %w", err)
	}
	if err := validateCapabilityBinding("webSearch", out.WebSearch); err != nil {
		return ToolBindings{}, err
	}
	if err := validateCapabilityBinding("fetchPage", out.FetchPage); err != nil {
		return ToolBindings{}, err
	}
	return out, nil
}

func validateToolBindingsPatch(raw json.RawMessage) error {
	if isJSONNull(raw) {
		return nil
	}
	if !isJSONObject(raw) {
		return fmt.Errorf("toolBindings must be an object")
	}
	var bag map[string]json.RawMessage
	if err := json.Unmarshal(raw, &bag); err != nil {
		return fmt.Errorf("toolBindings must be an object")
	}
	for key, value := range bag {
		switch key {
		case "webSearch", "fetchPage":
			if isJSONNull(value) {
				continue
			}
			var b CapabilityBinding
			if err := json.Unmarshal(value, &b); err != nil {
				return fmt.Errorf("toolBindings.%s must be an object", key)
			}
			if err := validateCapabilityBinding(key, b); err != nil {
				return err
			}
		default:
			return fmt.Errorf("toolBindings.%s is not supported", key)
		}
	}
	return nil
}

func validateCapabilityBinding(field string, b CapabilityBinding) error {
	mode := strings.TrimSpace(b.Mode)
	if mode == "" {
		mode = BindingInherit
	}
	switch mode {
	case BindingInherit, BindingDisabled:
		if strings.TrimSpace(b.IntegrationID) != "" {
			return fmt.Errorf("toolBindings.%s.integrationId must be empty when mode is %s", field, mode)
		}
	case BindingIntegration:
		if strings.TrimSpace(b.IntegrationID) == "" {
			return fmt.Errorf("toolBindings.%s.integrationId is required when mode is integration", field)
		}
	default:
		return fmt.Errorf("toolBindings.%s.mode must be inherit, disabled, or integration", field)
	}
	return nil
}

// ResolvedToolBinding is the plane-default + assistant-override result for one capability.
type ResolvedToolBinding struct {
	Capability    string
	Disabled      bool
	IntegrationID string
}

// ResolveToolBinding applies inherit / disabled / integration for one capability.
// planeDefaultID may be empty (no plane default configured).
func ResolveToolBinding(capability string, planeDefaultID string, binding CapabilityBinding) (ResolvedToolBinding, error) {
	mode := strings.TrimSpace(binding.Mode)
	if mode == "" {
		mode = BindingInherit
	}
	switch mode {
	case BindingDisabled:
		return ResolvedToolBinding{Capability: capability, Disabled: true}, nil
	case BindingIntegration:
		id := strings.TrimSpace(binding.IntegrationID)
		if id == "" {
			return ResolvedToolBinding{}, fmt.Errorf("%s binding requires integrationId", capability)
		}
		return ResolvedToolBinding{Capability: capability, IntegrationID: id}, nil
	case BindingInherit:
		id := strings.TrimSpace(planeDefaultID)
		if id == "" {
			return ResolvedToolBinding{Capability: capability, Disabled: true}, nil
		}
		return ResolvedToolBinding{Capability: capability, IntegrationID: id}, nil
	default:
		return ResolvedToolBinding{}, fmt.Errorf("invalid %s binding mode %q", capability, mode)
	}
}
