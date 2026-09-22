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

type ResolvedEnvironment struct {
	ResourceID    *string             `json:"resourceId"`
	Resource      *Resource           `json:"resource"`
	WorkspaceRoot string              `json:"workspaceRoot"`
	Volumes       []ResolvedEnvVolume `json:"volumes"`
	ExtraPaths    []PathRow           `json:"extraPaths"`
}

type ResolvedEnvVolume struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Target      string `json:"target"`
	Whitelisted bool   `json:"whitelisted"`
	Read        bool   `json:"read"`
	Write       bool   `json:"write"`
	Exec        bool   `json:"exec"`
}

type envGrantRow struct {
	VolumeID    string `json:"volumeId"`
	Whitelisted *bool  `json:"whitelisted,omitempty"`
	Read        *bool  `json:"read,omitempty"`
	Write       *bool  `json:"write,omitempty"`
	Exec        *bool  `json:"exec,omitempty"`
}

type flexVolume struct {
	ID          string `json:"id"`
	Enabled     *bool  `json:"enabled,omitempty"`
	Name        string `json:"name"`
	Target      string `json:"target"`
	Whitelisted *bool  `json:"whitelisted,omitempty"`
	Read        *bool  `json:"read,omitempty"`
	Write       *bool  `json:"write,omitempty"`
	Exec        *bool  `json:"exec,omitempty"`
}

func (s *Store) ResolveEnvironment(ctx context.Context, projectID string) (ResolvedEnvironment, error) {
	project, err := s.GetProject(ctx, projectID)
	if err != nil {
		return ResolvedEnvironment{}, err
	}
	globalSettings, err := s.GetPlaneSettings(ctx)
	if err != nil {
		return ResolvedEnvironment{}, err
	}
	projectEnv, err := EnvironmentFromSettings(project.Settings)
	if err != nil {
		return ResolvedEnvironment{}, err
	}
	globalEnv := globalSettings.Environment

	out := ResolvedEnvironment{
		WorkspaceRoot: environmentWorkspaceRoot(projectEnv, globalEnv),
		Volumes:       []ResolvedEnvVolume{},
		ExtraPaths:    []PathRow{},
	}

	resourceID := environmentResourceIDResolved(projectEnv, globalEnv)
	if resourceID == "" {
		return out, nil
	}
	id := resourceID
	out.ResourceID = &id

	resource, err := s.GetResource(ctx, resourceID)
	if err != nil {
		return ResolvedEnvironment{}, err
	}
	out.Resource = &resource

	volumes, err := resolveEnvironmentVolumes(resource.Spec, globalEnv, projectEnv)
	if err != nil {
		return ResolvedEnvironment{}, err
	}
	out.Volumes = volumes

	pathIndex := map[string]PathRow{}
	globalPaths, err := decodeEnvironmentExtraPaths(globalEnv)
	if err != nil {
		return ResolvedEnvironment{}, err
	}
	for _, row := range globalPaths {
		if row.ID == "" {
			continue
		}
		pathIndex[row.ID] = mergePathRow(pathIndex[row.ID], row)
	}
	projectPaths, err := decodeEnvironmentExtraPaths(projectEnv)
	if err != nil {
		return ResolvedEnvironment{}, err
	}
	for _, row := range projectPaths {
		if row.ID == "" {
			continue
		}
		pathIndex[row.ID] = mergePathRow(pathIndex[row.ID], row)
	}
	out.ExtraPaths = enabledPathRows(pathIndex)

	return out, nil
}

func environmentResourceIDResolved(projectEnv, globalEnv json.RawMessage) string {
	id, explicit := environmentResourceIDExplicit(projectEnv)
	if explicit && id != "" {
		return id
	}
	return environmentResourceID(globalEnv)
}

func environmentResourceIDExplicit(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 || isJSONNull(raw) {
		return "", false
	}
	var bag map[string]json.RawMessage
	if err := json.Unmarshal(raw, &bag); err != nil {
		return "", false
	}
	rawID, ok := bag["resourceId"]
	if !ok {
		return "", false
	}
	if isJSONNull(rawID) {
		return "", true
	}
	return strings.TrimSpace(stringFromRaw(rawID)), true
}

func environmentWorkspaceRoot(projectEnv, globalEnv json.RawMessage) string {
	if root := environmentStringField(projectEnv, "workspaceRoot"); root != "" {
		return root
	}
	if root := environmentStringField(globalEnv, "workspaceRoot"); root != "" {
		return root
	}
	return DefaultWorkspaceRoot
}

func environmentStringField(raw json.RawMessage, key string) string {
	if len(raw) == 0 || isJSONNull(raw) {
		return ""
	}
	var bag map[string]json.RawMessage
	if err := json.Unmarshal(raw, &bag); err != nil {
		return ""
	}
	val, ok := bag[key]
	if !ok || isJSONNull(val) {
		return ""
	}
	return strings.TrimSpace(stringFromRaw(val))
}

func decodeEnvironmentGrants(raw json.RawMessage) ([]envGrantRow, error) {
	if len(raw) == 0 || isJSONNull(raw) {
		return nil, nil
	}
	var bag map[string]json.RawMessage
	if err := json.Unmarshal(raw, &bag); err != nil {
		return nil, fmt.Errorf("decode environment: %w", err)
	}
	grantsRaw, ok := bag["grants"]
	if !ok || isJSONNull(grantsRaw) {
		return nil, nil
	}
	var rows []envGrantRow
	if err := json.Unmarshal(grantsRaw, &rows); err != nil {
		return nil, fmt.Errorf("decode grants: %w", err)
	}
	return rows, nil
}

func decodeEnvironmentExtraPaths(raw json.RawMessage) ([]PathRow, error) {
	if len(raw) == 0 || isJSONNull(raw) {
		return nil, nil
	}
	var bag map[string]json.RawMessage
	if err := json.Unmarshal(raw, &bag); err != nil {
		return nil, fmt.Errorf("decode environment: %w", err)
	}
	pathsRaw, ok := bag["extraPaths"]
	if !ok || isJSONNull(pathsRaw) {
		return nil, nil
	}
	var rows []PathRow
	if err := json.Unmarshal(pathsRaw, &rows); err != nil {
		return nil, fmt.Errorf("decode extraPaths: %w", err)
	}
	return rows, nil
}

func volumesFromResourceSpec(spec json.RawMessage) ([]flexVolume, error) {
	var bag struct {
		Volumes []flexVolume `json:"volumes"`
	}
	if len(spec) == 0 {
		return nil, nil
	}
	if err := json.Unmarshal(spec, &bag); err != nil {
		return nil, fmt.Errorf("decode resource spec volumes: %w", err)
	}
	return bag.Volumes, nil
}

func resolveEnvironmentVolumes(spec, globalEnv, projectEnv json.RawMessage) ([]ResolvedEnvVolume, error) {
	flexVols, err := volumesFromResourceSpec(spec)
	if err != nil {
		return nil, err
	}
	index := map[string]ResolvedEnvVolume{}
	order := make([]string, 0, len(flexVols))
	for _, vol := range flexVols {
		if vol.ID == "" {
			continue
		}
		if !boolOrDefault(vol.Enabled, true) {
			continue
		}
		index[vol.ID] = ResolvedEnvVolume{
			ID:          vol.ID,
			Name:        vol.Name,
			Target:      vol.Target,
			Whitelisted: boolOrDefault(vol.Whitelisted, true),
			Read:        boolOrDefault(vol.Read, true),
			Write:       boolOrDefault(vol.Write, true),
			Exec:        boolOrDefault(vol.Exec, true),
		}
		order = append(order, vol.ID)
	}

	globalGrants, err := decodeEnvironmentGrants(globalEnv)
	if err != nil {
		return nil, err
	}
	for _, grant := range globalGrants {
		vol, ok := index[grant.VolumeID]
		if !ok {
			continue
		}
		applyEnvGrant(&vol, grant)
		index[grant.VolumeID] = vol
	}
	projectGrants, err := decodeEnvironmentGrants(projectEnv)
	if err != nil {
		return nil, err
	}
	for _, grant := range projectGrants {
		vol, ok := index[grant.VolumeID]
		if !ok {
			continue
		}
		applyEnvGrant(&vol, grant)
		index[grant.VolumeID] = vol
	}

	out := make([]ResolvedEnvVolume, 0, len(order))
	for _, id := range order {
		if vol, ok := index[id]; ok {
			out = append(out, vol)
		}
	}
	return out, nil
}

func applyEnvGrant(vol *ResolvedEnvVolume, grant envGrantRow) {
	if grant.Whitelisted != nil {
		vol.Whitelisted = *grant.Whitelisted
	}
	if grant.Read != nil {
		vol.Read = *grant.Read
	}
	if grant.Write != nil {
		vol.Write = *grant.Write
	}
	if grant.Exec != nil {
		vol.Exec = *grant.Exec
	}
}

func boolOrDefault(value *bool, def bool) bool {
	if value == nil {
		return def
	}
	return *value
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
