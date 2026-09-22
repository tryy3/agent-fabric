package catalog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
)

const (
	planeSettingsID              = "default"
	WorkspaceVolumeID            = "vol_workspace"
	DefaultSandboxKind           = "docker"
	DefaultWorkspaceRoot         = "/workspace"
	DefaultSandboxImage          = "alpine:3.20"
	DefaultIdleTTLSeconds        = int64(3600)
	DefaultContainerNameTemplate = "agent-fabric-container-{projectID}"
	DefaultVolumeNameTemplate    = "agent-fabric-vol-{projectID}"
	Phase1VolumeNameTemplate     = "agent-fabric.proj.{projectID}"
)

type Overlay struct {
	Kind           *string     `json:"kind,omitempty"`
	WorkspaceRoot  *string     `json:"workspaceRoot,omitempty"`
	Image          *string     `json:"image,omitempty"`
	IdleTTLSeconds *int64      `json:"idleTTLSeconds,omitempty"`
	Dockerfile     *string     `json:"dockerfile,omitempty"`
	BuildContext   *string     `json:"buildContext,omitempty"`
	ContainerName  *string     `json:"containerName,omitempty"`
	Volumes        []VolumeRow `json:"volumes,omitempty"`
	ExtraPaths     []PathRow   `json:"extraPaths,omitempty"`
}

type VolumeRow struct {
	ID          string  `json:"id"`
	Enabled     *bool   `json:"enabled,omitempty"`
	Name        *string `json:"name,omitempty"`
	Target      *string `json:"target,omitempty"`
	Whitelisted *bool   `json:"whitelisted,omitempty"`
	Read        *bool   `json:"read,omitempty"`
	Write       *bool   `json:"write,omitempty"`
	Exec        *bool   `json:"exec,omitempty"`
}

type PathRow struct {
	ID          string  `json:"id"`
	Enabled     *bool   `json:"enabled,omitempty"`
	Path        *string `json:"path,omitempty"`
	Whitelisted *bool   `json:"whitelisted,omitempty"`
	Read        *bool   `json:"read,omitempty"`
	Write       *bool   `json:"write,omitempty"`
	Exec        *bool   `json:"exec,omitempty"`
}

type PlaneSettings struct {
	Sandbox     json.RawMessage `json:"sandbox"`
	Environment json.RawMessage `json:"environment"`
}

func DefaultOverlay(preservePhase1Volumes bool) Overlay {
	kind := DefaultSandboxKind
	root := DefaultWorkspaceRoot
	image := DefaultSandboxImage
	ttl := DefaultIdleTTLSeconds
	container := DefaultContainerNameTemplate
	volName := DefaultVolumeNameTemplate
	if preservePhase1Volumes {
		volName = Phase1VolumeNameTemplate
	}
	enabled := true
	return Overlay{
		Kind:           &kind,
		WorkspaceRoot:  &root,
		Image:          &image,
		IdleTTLSeconds: &ttl,
		ContainerName:  &container,
		Volumes: []VolumeRow{{
			ID:          WorkspaceVolumeID,
			Enabled:     &enabled,
			Name:        &volName,
			Target:      &root,
			Whitelisted: &enabled,
			Read:        &enabled,
			Write:       &enabled,
			Exec:        &enabled,
		}},
	}
}

func DecodeOverlay(raw json.RawMessage) (Overlay, error) {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return Overlay{}, nil
	}
	var overlay Overlay
	if err := json.Unmarshal(raw, &overlay); err != nil {
		return Overlay{}, fmt.Errorf("decode sandbox overlay: %w", err)
	}
	return overlay, nil
}

func EncodeOverlay(overlay Overlay) (json.RawMessage, error) {
	raw, err := json.Marshal(overlay)
	if err != nil {
		return nil, fmt.Errorf("encode sandbox overlay: %w", err)
	}
	return raw, nil
}

func SandboxFromSettings(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return json.RawMessage(`{}`), nil
	}
	var bag map[string]json.RawMessage
	if err := json.Unmarshal(raw, &bag); err != nil {
		return nil, fmt.Errorf("decode settings: %w", err)
	}
	if sandbox, ok := bag["sandbox"]; ok && len(sandbox) > 0 {
		return sandbox, nil
	}
	return json.RawMessage(`{}`), nil
}

func PatchOverlayJSON(base, patch json.RawMessage) (json.RawMessage, error) {
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
		case "volumes", "extraPaths":
			if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
				delete(baseMap, key)
				continue
			}
			merged, mergeErr := patchKeyedRows(baseMap[key], value, key)
			if mergeErr != nil {
				return nil, mergeErr
			}
			baseMap[key] = merged
		default:
			if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
				delete(baseMap, key)
				continue
			}
			baseMap[key] = value
		}
	}
	out, err := json.Marshal(baseMap)
	if err != nil {
		return nil, fmt.Errorf("encode patched overlay: %w", err)
	}
	return out, nil
}

func MergeSettingsSandbox(settings, sandboxPatch json.RawMessage) (json.RawMessage, error) {
	if len(sandboxPatch) == 0 {
		return MergeSettings(settings, nil)
	}
	patch, err := json.Marshal(map[string]json.RawMessage{"sandbox": sandboxPatch})
	if err != nil {
		return nil, fmt.Errorf("encode sandbox patch: %w", err)
	}
	return MergeSettings(settings, patch)
}

// MergeSettings JSON-merge-patches a settings object. sandbox uses keyed overlay
// merge; other nested objects merge recursively; arrays and scalars replace.
// A null value deletes that key. Unspecified keys are left in place.
func MergeSettings(settings, patch json.RawMessage) (json.RawMessage, error) {
	bag := map[string]json.RawMessage{}
	if len(settings) > 0 && !bytes.Equal(bytes.TrimSpace(settings), []byte("null")) {
		if err := json.Unmarshal(settings, &bag); err != nil {
			return nil, fmt.Errorf("decode settings: %w", err)
		}
	}
	if len(patch) == 0 || bytes.Equal(bytes.TrimSpace(patch), []byte("null")) {
		out, err := json.Marshal(bag)
		if err != nil {
			return nil, fmt.Errorf("encode settings: %w", err)
		}
		return out, nil
	}
	patchBag, err := overlayMap(patch)
	if err != nil {
		return nil, fmt.Errorf("decode settings patch: %w", err)
	}
	if err := validateSettingsPatch(patchBag); err != nil {
		return nil, err
	}
	for key, value := range patchBag {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			delete(bag, key)
			continue
		}
		if key == "sandbox" {
			current := bag["sandbox"]
			if len(current) == 0 {
				current = json.RawMessage(`{}`)
			}
			patched, patchErr := PatchOverlayJSON(current, value)
			if patchErr != nil {
				return nil, patchErr
			}
			bag["sandbox"] = patched
			continue
		}
		if key == "environment" {
			current := bag["environment"]
			if len(current) == 0 {
				current = json.RawMessage(`{}`)
			}
			patched, patchErr := PatchEnvironmentJSON(current, value)
			if patchErr != nil {
				return nil, patchErr
			}
			bag["environment"] = patched
			continue
		}
		merged, mergeErr := mergeJSONValue(bag[key], value)
		if mergeErr != nil {
			return nil, fmt.Errorf("merge settings.%s: %w", key, mergeErr)
		}
		if merged == nil {
			delete(bag, key)
			continue
		}
		bag[key] = merged
	}
	out, err := json.Marshal(bag)
	if err != nil {
		return nil, fmt.Errorf("encode settings: %w", err)
	}
	return out, nil
}

func validateSettingsPatch(patch map[string]json.RawMessage) error {
	if raw, ok := patch["allowedAgents"]; ok && !isJSONNull(raw) {
		var ids []string
		if err := json.Unmarshal(raw, &ids); err != nil {
			return fmt.Errorf("allowedAgents must be an array of agent ids")
		}
	}
	if raw, ok := patch["tools"]; ok && !isJSONNull(raw) {
		if !isJSONObject(raw) {
			return fmt.Errorf("tools must be an object")
		}
		var tools map[string]json.RawMessage
		if err := json.Unmarshal(raw, &tools); err != nil {
			return fmt.Errorf("tools must be an object")
		}
		if allow, ok := tools["allow"]; ok && !isJSONNull(allow) {
			var names []string
			if err := json.Unmarshal(allow, &names); err != nil {
				return fmt.Errorf("tools.allow must be an array of tool names")
			}
		}
	}
	if raw, ok := patch["mcp"]; ok && !isJSONNull(raw) {
		if err := validateStubObjectArray(raw, "mcp", "servers"); err != nil {
			return err
		}
	}
	if raw, ok := patch["memory"]; ok && !isJSONNull(raw) {
		if !isJSONObject(raw) {
			return fmt.Errorf("memory must be an object")
		}
	}
	if raw, ok := patch["context"]; ok && !isJSONNull(raw) {
		if err := validateStubObjectArray(raw, "context", "items"); err != nil {
			return err
		}
	}
	return nil
}

func validateStubObjectArray(raw json.RawMessage, group, field string) error {
	if !isJSONObject(raw) {
		return fmt.Errorf("%s must be an object", group)
	}
	var bag map[string]json.RawMessage
	if err := json.Unmarshal(raw, &bag); err != nil {
		return fmt.Errorf("%s must be an object", group)
	}
	items, ok := bag[field]
	if !ok || isJSONNull(items) {
		return nil
	}
	trimmed := bytes.TrimSpace(items)
	if len(trimmed) == 0 || trimmed[0] != '[' {
		return fmt.Errorf("%s.%s must be an array", group, field)
	}
	return nil
}

func mergeJSONValue(base, patch json.RawMessage) (json.RawMessage, error) {
	if isJSONNull(patch) {
		return nil, nil
	}
	if len(patch) == 0 {
		return base, nil
	}
	if isJSONObject(patch) {
		baseMap := map[string]json.RawMessage{}
		if isJSONObject(base) {
			if err := json.Unmarshal(base, &baseMap); err != nil {
				return nil, err
			}
		}
		patchMap := map[string]json.RawMessage{}
		if err := json.Unmarshal(patch, &patchMap); err != nil {
			return nil, err
		}
		for key, value := range patchMap {
			if isJSONNull(value) {
				delete(baseMap, key)
				continue
			}
			merged, err := mergeJSONValue(baseMap[key], value)
			if err != nil {
				return nil, err
			}
			if merged == nil {
				delete(baseMap, key)
				continue
			}
			baseMap[key] = merged
		}
		return json.Marshal(baseMap)
	}
	out := make(json.RawMessage, len(patch))
	copy(out, patch)
	return out, nil
}

func isJSONNull(raw json.RawMessage) bool {
	return len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

func isJSONObject(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) > 0 && trimmed[0] == '{'
}

func PatchRemotesJSON(base, patch json.RawMessage) (json.RawMessage, error) {
	if isJSONNull(patch) {
		return json.RawMessage(`[]`), nil
	}
	trimmed := bytes.TrimSpace(patch)
	if len(trimmed) > 0 && trimmed[0] == '{' {
		return nil, fmt.Errorf("remotes must be an array")
	}
	return patchKeyedRows(base, patch, "remotes")
}

func ResolveOverlay(layers ...Overlay) Overlay {
	out := Overlay{}
	volumeIndex := map[string]VolumeRow{}
	pathIndex := map[string]PathRow{}
	for _, layer := range layers {
		replaceString(&out.Kind, layer.Kind)
		replaceString(&out.WorkspaceRoot, layer.WorkspaceRoot)
		replaceString(&out.Image, layer.Image)
		if layer.IdleTTLSeconds != nil {
			ttl := *layer.IdleTTLSeconds
			out.IdleTTLSeconds = &ttl
		}
		replaceString(&out.Dockerfile, layer.Dockerfile)
		replaceString(&out.BuildContext, layer.BuildContext)
		replaceString(&out.ContainerName, layer.ContainerName)
		for _, row := range layer.Volumes {
			if row.ID == "" {
				continue
			}
			volumeIndex[row.ID] = mergeVolumeRow(volumeIndex[row.ID], row)
		}
		for _, row := range layer.ExtraPaths {
			if row.ID == "" {
				continue
			}
			pathIndex[row.ID] = mergePathRow(pathIndex[row.ID], row)
		}
	}
	out.Volumes = enabledVolumeRows(volumeIndex)
	out.ExtraPaths = enabledPathRows(pathIndex)
	return out
}

func overlayMap(raw json.RawMessage) (map[string]json.RawMessage, error) {
	out := map[string]json.RawMessage{}
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return out, nil
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode sandbox overlay: %w", err)
	}
	return out, nil
}

func patchKeyedRows(base, patch json.RawMessage, field string) (json.RawMessage, error) {
	baseRows, err := decodeIDMaps(base)
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", field, err)
	}
	patchRows, err := decodeIDMaps(patch)
	if err != nil {
		return nil, fmt.Errorf("decode %s patch: %w", field, err)
	}
	order := make([]string, 0, len(baseRows)+len(patchRows))
	seen := map[string]struct{}{}
	for _, row := range baseRows {
		id := stringFromRaw(row["id"])
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
		id := stringFromRaw(row["id"])
		if id != "" {
			index[id] = row
		}
	}
	for _, row := range patchRows {
		id := stringFromRaw(row["id"])
		if id == "" {
			return nil, fmt.Errorf("%s entry is missing id", field)
		}
		if field == "remotes" {
			kind := stringFromRaw(row["kind"])
			if kind != "" && kind != "github" && kind != "s3" {
				return nil, fmt.Errorf("remote kind must be github or s3")
			}
		}
		if _, ok := seen[id]; !ok {
			order = append(order, id)
			seen[id] = struct{}{}
		}
		current := index[id]
		if current == nil {
			current = map[string]json.RawMessage{"id": row["id"]}
		}
		for key, value := range row {
			if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
				delete(current, key)
				continue
			}
			current[key] = value
		}
		index[id] = current
	}
	merged := make([]map[string]json.RawMessage, 0, len(order))
	for _, id := range order {
		merged = append(merged, index[id])
	}
	raw, err := json.Marshal(merged)
	if err != nil {
		return nil, fmt.Errorf("encode %s: %w", field, err)
	}
	return raw, nil
}

func decodeIDMaps(raw json.RawMessage) ([]map[string]json.RawMessage, error) {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, nil
	}
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func stringFromRaw(raw json.RawMessage) string {
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return ""
	}
	return value
}

func replaceString(dst **string, src *string) {
	if src == nil {
		return
	}
	value := *src
	*dst = &value
}

func replaceBool(dst **bool, src *bool) {
	if src == nil {
		return
	}
	value := *src
	*dst = &value
}

func mergeVolumeRow(base, layer VolumeRow) VolumeRow {
	out := base
	if out.ID == "" {
		out.ID = layer.ID
	}
	replaceBool(&out.Enabled, layer.Enabled)
	replaceString(&out.Name, layer.Name)
	replaceString(&out.Target, layer.Target)
	out.Whitelisted = denyWinsBool(out.Whitelisted, layer.Whitelisted)
	out.Read = denyWinsBool(out.Read, layer.Read)
	out.Write = denyWinsBool(out.Write, layer.Write)
	out.Exec = denyWinsBool(out.Exec, layer.Exec)
	return out
}

func mergePathRow(base, layer PathRow) PathRow {
	out := base
	if out.ID == "" {
		out.ID = layer.ID
	}
	replaceBool(&out.Enabled, layer.Enabled)
	replaceString(&out.Path, layer.Path)
	out.Whitelisted = denyWinsBool(out.Whitelisted, layer.Whitelisted)
	out.Read = denyWinsBool(out.Read, layer.Read)
	out.Write = denyWinsBool(out.Write, layer.Write)
	out.Exec = denyWinsBool(out.Exec, layer.Exec)
	return out
}

func denyWinsBool(base, layer *bool) *bool {
	if layer == nil {
		return cloneBool(base)
	}
	if base != nil && !*base {
		falseVal := false
		return &falseVal
	}
	value := *layer
	return &value
}

func cloneBool(value *bool) *bool {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func enabledVolumeRows(index map[string]VolumeRow) []VolumeRow {
	out := make([]VolumeRow, 0, len(index))
	if row, ok := index[WorkspaceVolumeID]; ok {
		if row.Enabled == nil || *row.Enabled {
			out = append(out, row)
		}
	}
	ids := make([]string, 0, len(index))
	for id := range index {
		if id == WorkspaceVolumeID {
			continue
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		row := index[id]
		if row.Enabled != nil && !*row.Enabled {
			continue
		}
		out = append(out, row)
	}
	return out
}

func enabledPathRows(index map[string]PathRow) []PathRow {
	ids := make([]string, 0, len(index))
	for id := range index {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]PathRow, 0, len(ids))
	for _, id := range ids {
		row := index[id]
		if row.Enabled != nil && !*row.Enabled {
			continue
		}
		out = append(out, row)
	}
	return out
}
