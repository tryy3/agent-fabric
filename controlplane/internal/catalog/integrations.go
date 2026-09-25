package catalog

import (
	"encoding/json"
	"fmt"
)

// PatchIntegrationsJSON shallow-merges top-level integration keys. Nested
// objects (e.g. netlify) are replaced wholesale; null deletes a key.
func PatchIntegrationsJSON(base, patch json.RawMessage) (json.RawMessage, error) {
	baseMap, err := overlayMap(base)
	if err != nil {
		return nil, fmt.Errorf("decode integrations: %w", err)
	}
	patchMap, err := overlayMap(patch)
	if err != nil {
		return nil, fmt.Errorf("decode integrations patch: %w", err)
	}
	for key, value := range patchMap {
		if isJSONNull(value) {
			delete(baseMap, key)
			continue
		}
		if isJSONObject(value) {
			current := baseMap[key]
			if len(current) == 0 || !isJSONObject(current) {
				current = json.RawMessage(`{}`)
			}
			merged, mergeErr := mergeObjectShallow(current, value)
			if mergeErr != nil {
				return nil, mergeErr
			}
			baseMap[key] = merged
			continue
		}
		baseMap[key] = value
	}
	out, err := json.Marshal(baseMap)
	if err != nil {
		return nil, fmt.Errorf("encode integrations: %w", err)
	}
	return out, nil
}

func mergeObjectShallow(base, patch json.RawMessage) (json.RawMessage, error) {
	baseMap, err := overlayMap(base)
	if err != nil {
		return nil, err
	}
	patchMap, err := overlayMap(patch)
	if err != nil {
		return nil, err
	}
	for key, value := range patchMap {
		if isJSONNull(value) {
			delete(baseMap, key)
			continue
		}
		baseMap[key] = value
	}
	return json.Marshal(baseMap)
}
