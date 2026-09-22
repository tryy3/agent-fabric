package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

func EnvironmentFromSettings(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return json.RawMessage(`{}`), nil
	}
	var bag map[string]json.RawMessage
	if err := json.Unmarshal(raw, &bag); err != nil {
		return nil, fmt.Errorf("decode settings: %w", err)
	}
	if env, ok := bag["environment"]; ok && len(env) > 0 && !isJSONNull(env) {
		return env, nil
	}
	return json.RawMessage(`{}`), nil
}

func PatchEnvironmentJSON(base, patch json.RawMessage) (json.RawMessage, error) {
	baseMap, err := overlayMap(base)
	if err != nil {
		return nil, err
	}
	patchMap, err := overlayMap(patch)
	if err != nil {
		return nil, err
	}
	for key, value := range patchMap {
		switch key {
		case "grants":
			if isJSONNull(value) {
				delete(baseMap, key)
				continue
			}
			merged, mergeErr := patchKeyedRowsByField(baseMap[key], value, "grants", "volumeId")
			if mergeErr != nil {
				return nil, mergeErr
			}
			baseMap[key] = merged
		case "extraPaths":
			if isJSONNull(value) {
				delete(baseMap, key)
				continue
			}
			merged, mergeErr := patchKeyedRows(baseMap[key], value, "extraPaths")
			if mergeErr != nil {
				return nil, mergeErr
			}
			baseMap[key] = merged
		default:
			if isJSONNull(value) {
				delete(baseMap, key)
				continue
			}
			baseMap[key] = value
		}
	}
	out, err := json.Marshal(baseMap)
	if err != nil {
		return nil, fmt.Errorf("encode patched environment: %w", err)
	}
	return out, nil
}

func patchKeyedRowsByField(base, patch json.RawMessage, field, keyField string) (json.RawMessage, error) {
	baseRows, err := decodeIDMaps(base)
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", field, err)
	}
	var patchElems []json.RawMessage
	if len(patch) > 0 && !isJSONNull(patch) {
		if err := json.Unmarshal(patch, &patchElems); err != nil {
			return nil, fmt.Errorf("decode %s patch: %w", field, err)
		}
	}

	order := make([]string, 0, len(baseRows))
	seen := map[string]struct{}{}
	for _, row := range baseRows {
		id := stringFromRaw(row[keyField])
		if id == "" {
			continue
		}
		if _, ok := seen[id]; !ok {
			order = append(order, id)
			seen[id] = struct{}{}
		}
	}
	index := map[string]map[string]json.RawMessage{}
	for _, row := range baseRows {
		id := stringFromRaw(row[keyField])
		if id != "" {
			index[id] = row
		}
	}

	rowList := make([]map[string]json.RawMessage, 0, len(baseRows))
	for _, id := range order {
		rowList = append(rowList, index[id])
	}

	if len(patchElems) == 1 && isJSONNull(patchElems[0]) {
		return json.RawMessage(`[]`), nil
	}

	for patchIdx, elem := range patchElems {
		if isJSONNull(elem) {
			if patchIdx < len(rowList) {
				rowList = append(rowList[:patchIdx], rowList[patchIdx+1:]...)
				order = orderFromRows(rowList, keyField)
				index = indexFromRows(rowList, keyField)
			}
			continue
		}
		var row map[string]json.RawMessage
		if err := json.Unmarshal(elem, &row); err != nil {
			return nil, fmt.Errorf("decode %s patch row: %w", field, err)
		}
		id := stringFromRaw(row[keyField])
		if id == "" {
			return nil, fmt.Errorf("%s entry is missing %s", field, keyField)
		}
		if _, ok := seen[id]; !ok {
			order = append(order, id)
			seen[id] = struct{}{}
		}
		current := index[id]
		if current == nil {
			current = map[string]json.RawMessage{keyField: row[keyField]}
		}
		for key, value := range row {
			if isJSONNull(value) {
				delete(current, key)
				continue
			}
			current[key] = value
		}
		index[id] = current
	}

	merged := make([]map[string]json.RawMessage, 0, len(order))
	for _, id := range order {
		if row, ok := index[id]; ok {
			merged = append(merged, row)
		}
	}
	raw, err := json.Marshal(merged)
	if err != nil {
		return nil, fmt.Errorf("encode %s: %w", field, err)
	}
	return raw, nil
}

func orderFromRows(rows []map[string]json.RawMessage, keyField string) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		id := stringFromRaw(row[keyField])
		if id != "" {
			out = append(out, id)
		}
	}
	return out
}

func indexFromRows(rows []map[string]json.RawMessage, keyField string) map[string]map[string]json.RawMessage {
	out := map[string]map[string]json.RawMessage{}
	for _, row := range rows {
		id := stringFromRaw(row[keyField])
		if id != "" {
			out[id] = row
		}
	}
	return out
}

func environmentResourceID(raw json.RawMessage) string {
	if len(raw) == 0 || isJSONNull(raw) {
		return ""
	}
	var bag map[string]json.RawMessage
	if err := json.Unmarshal(raw, &bag); err != nil {
		return ""
	}
	rawID, ok := bag["resourceId"]
	if !ok || isJSONNull(rawID) {
		return ""
	}
	return strings.TrimSpace(stringFromRaw(rawID))
}

func resolveEnvironmentResourceID(envPatch, storedEnv, globalEnv json.RawMessage) string {
	if len(envPatch) > 0 && !isJSONNull(envPatch) {
		var patchBag map[string]json.RawMessage
		if err := json.Unmarshal(envPatch, &patchBag); err == nil {
			if raw, ok := patchBag["resourceId"]; ok {
				if isJSONNull(raw) {
					return ""
				}
				if id := strings.TrimSpace(stringFromRaw(raw)); id != "" {
					return id
				}
			}
		}
	}
	if id := environmentResourceID(storedEnv); id != "" {
		return id
	}
	return environmentResourceID(globalEnv)
}

func ValidateEnvironmentPatch(_ context.Context, resourceSpec json.RawMessage, patch json.RawMessage) error {
	if len(patch) == 0 || isJSONNull(patch) {
		return nil
	}
	patchMap, err := overlayMap(patch)
	if err != nil {
		return err
	}
	grantsRaw, hasGrants := patchMap["grants"]
	if !hasGrants || isJSONNull(grantsRaw) {
		return nil
	}
	if len(resourceSpec) == 0 || isJSONNull(resourceSpec) {
		return fmt.Errorf("no resource selected")
	}
	spec, err := decodeContainerSpec(resourceSpec)
	if err != nil {
		return err
	}
	volumeIDs := map[string]struct{}{}
	for _, vol := range spec.Volumes {
		volumeIDs[vol.ID] = struct{}{}
	}
	var grantElems []json.RawMessage
	if err := json.Unmarshal(grantsRaw, &grantElems); err != nil {
		return fmt.Errorf("decode grants: %w", err)
	}
	for _, elem := range grantElems {
		if isJSONNull(elem) {
			continue
		}
		var row map[string]json.RawMessage
		if err := json.Unmarshal(elem, &row); err != nil {
			return fmt.Errorf("decode grants: %w", err)
		}
		volumeID := stringFromRaw(row["volumeId"])
		if volumeID == "" {
			continue
		}
		if _, ok := volumeIDs[volumeID]; !ok {
			return fmt.Errorf("unknown volume %q", volumeID)
		}
	}
	return nil
}

func (s *Store) validateEnvironmentPatch(ctx context.Context, envPatch, storedEnv, globalEnv json.RawMessage) error {
	if len(envPatch) == 0 || isJSONNull(envPatch) {
		return nil
	}
	patchMap, err := overlayMap(envPatch)
	if err != nil {
		return err
	}
	if raw, ok := patchMap["resourceId"]; ok && !isJSONNull(raw) {
		id := strings.TrimSpace(stringFromRaw(raw))
		if id != "" {
			if _, err := s.GetResource(ctx, id); err != nil {
				return err
			}
		}
	}
	resourceID := resolveEnvironmentResourceID(envPatch, storedEnv, globalEnv)
	if resourceID == "" {
		if _, hasGrants := patchMap["grants"]; hasGrants && !isJSONNull(patchMap["grants"]) {
			return fmt.Errorf("no resource selected")
		}
		return nil
	}
	resource, err := s.GetResource(ctx, resourceID)
	if err != nil {
		return err
	}
	return ValidateEnvironmentPatch(ctx, resource.Spec, envPatch)
}

func (s *Store) resourceLinkedInEnvironment(id string, env json.RawMessage) bool {
	return environmentResourceID(env) == id
}

func (s *Store) resourceInUse(ctx context.Context, id string) (bool, error) {
	settings, err := s.GetPlaneSettings(ctx)
	if err != nil {
		return false, err
	}
	if s.resourceLinkedInEnvironment(id, settings.Environment) {
		return true, nil
	}
	projects, err := s.ListProjects(ctx)
	if err != nil {
		return false, err
	}
	for _, project := range projects {
		env, err := EnvironmentFromSettings(project.Settings)
		if err != nil {
			return false, err
		}
		if s.resourceLinkedInEnvironment(id, env) {
			return true, nil
		}
	}
	return false, nil
}

func patchHasEnvironment(raw json.RawMessage) bool {
	if len(raw) == 0 || isJSONNull(raw) {
		return false
	}
	var bag map[string]json.RawMessage
	if err := json.Unmarshal(raw, &bag); err != nil {
		return false
	}
	_, ok := bag["environment"]
	return ok
}
