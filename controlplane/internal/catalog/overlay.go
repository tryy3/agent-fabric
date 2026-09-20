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
	Sandbox json.RawMessage `json:"sandbox"`
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
	bag := map[string]json.RawMessage{}
	if len(settings) > 0 {
		if err := json.Unmarshal(settings, &bag); err != nil {
			return nil, fmt.Errorf("decode settings: %w", err)
		}
	}
	current := bag["sandbox"]
	if len(current) == 0 {
		current = json.RawMessage(`{}`)
	}
	patched, err := PatchOverlayJSON(current, sandboxPatch)
	if err != nil {
		return nil, err
	}
	bag["sandbox"] = patched
	out, err := json.Marshal(bag)
	if err != nil {
		return nil, fmt.Errorf("encode settings: %w", err)
	}
	return out, nil
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
