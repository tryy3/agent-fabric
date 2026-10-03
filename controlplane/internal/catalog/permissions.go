package catalog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// Permissions holds agent settings.permissions. Mode is one of the gate
// permission modes; empty means the default (ask).
type Permissions struct {
	Mode string `json:"mode,omitempty"`
}

// PermissionsFromSettings extracts settings.permissions from an agent settings blob.
func PermissionsFromSettings(settings json.RawMessage) (Permissions, error) {
	if len(settings) == 0 || bytes.Equal(bytes.TrimSpace(settings), []byte("null")) {
		return Permissions{}, nil
	}
	var bag map[string]json.RawMessage
	if err := json.Unmarshal(settings, &bag); err != nil {
		return Permissions{}, fmt.Errorf("decode settings: %w", err)
	}
	raw, ok := bag["permissions"]
	if !ok || isJSONNull(raw) {
		return Permissions{}, nil
	}
	return DecodePermissions(raw)
}

// DecodePermissions validates and decodes a permissions JSON object.
func DecodePermissions(raw json.RawMessage) (Permissions, error) {
	if !isJSONObject(raw) {
		return Permissions{}, fmt.Errorf("permissions must be an object")
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(raw, &keys); err != nil {
		return Permissions{}, fmt.Errorf("permissions must be an object")
	}
	for key := range keys {
		if key != "mode" {
			return Permissions{}, fmt.Errorf("permissions: unknown key %q", key)
		}
	}
	var p Permissions
	if err := json.Unmarshal(raw, &p); err != nil {
		return Permissions{}, fmt.Errorf("decode permissions: %w", err)
	}
	if p.Mode != "" && !slices.Contains(PermissionModes, p.Mode) {
		return Permissions{}, fmt.Errorf("permissions.mode must be one of %s", strings.Join(PermissionModes, ", "))
	}
	return p, nil
}

// PermissionModes lists the valid settings.permissions.mode values, least to
// most permissive. It mirrors gate.Modes (a gate test keeps them in sync); the
// catalog cannot import the gate without a cycle through the provider package.
var PermissionModes = []string{"ask", "auto_approve", "auto", "full"}
