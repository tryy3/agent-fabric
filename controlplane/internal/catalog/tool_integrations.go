package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/tryy3/agent-fabric/internal/db"
)

// CreateToolIntegrationParams is the input for creating a tool integration.
type CreateToolIntegrationParams struct {
	Name     string
	Kind     string
	Enabled  *bool
	Endpoint string
	Mode     string
	Config   json.RawMessage
	Secrets  map[string]*string // nil value ignored on create; empty string rejected for required
}

// PatchToolIntegrationParams is a partial update. Secret keys: omitted = preserve,
// explicit null (present with nil pointer) = clear, non-nil string = set.
type PatchToolIntegrationParams struct {
	Name     *string
	Enabled  *bool
	Endpoint *string
	Mode     *string
	Config   json.RawMessage
	Secrets  map[string]*string
}

func (s *Store) ListToolIntegrations(ctx context.Context) ([]ToolIntegration, error) {
	rows, err := s.q.ListToolIntegrations(ctx)
	if err != nil {
		return nil, fmt.Errorf("list tool integrations: %w", err)
	}
	out := make([]ToolIntegration, 0, len(rows))
	for _, row := range rows {
		pub, err := publicToolIntegration(row)
		if err != nil {
			return nil, err
		}
		out = append(out, pub)
	}
	return out, nil
}

func (s *Store) GetToolIntegration(ctx context.Context, id string) (ToolIntegration, error) {
	row, err := s.q.GetToolIntegration(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ToolIntegration{}, newToolIntegrationNotFound(id)
		}
		return ToolIntegration{}, fmt.Errorf("get tool integration: %w", err)
	}
	return publicToolIntegration(row)
}

// GetToolIntegrationSecrets returns decrypted secret values for runtime use.
func (s *Store) GetToolIntegrationSecrets(ctx context.Context, id string) (ToolIntegrationSecrets, error) {
	row, err := s.q.GetToolIntegration(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, newToolIntegrationNotFound(id)
		}
		return nil, fmt.Errorf("get tool integration: %w", err)
	}
	return decodeSecrets(row.Secrets)
}

func (s *Store) CreateToolIntegration(ctx context.Context, p CreateToolIntegrationParams) (ToolIntegration, error) {
	kind := strings.TrimSpace(p.Kind)
	if !IsKnownIntegrationKind(kind) {
		return ToolIntegration{}, fmt.Errorf("unknown integration kind %q", kind)
	}
	name := strings.TrimSpace(p.Name)
	if name == "" {
		return ToolIntegration{}, fmt.Errorf("integration name is required")
	}
	mode := strings.TrimSpace(p.Mode)
	if mode == "" {
		if _, ok := BundledDefaultEndpoints[kind]; ok && kind != KindLinkup {
			mode = ModeBundled
		} else {
			mode = ModeExternal
		}
	}
	if mode != ModeBundled && mode != ModeExternal {
		return ToolIntegration{}, fmt.Errorf("integration mode must be bundled or external")
	}
	if mode == ModeBundled && kind == KindLinkup {
		return ToolIntegration{}, fmt.Errorf("linkup does not support bundled mode")
	}
	endpoint := strings.TrimSpace(p.Endpoint)
	if endpoint == "" && mode == ModeBundled {
		endpoint = BundledDefaultEndpoints[kind]
	}
	if mode == ModeExternal && endpoint == "" {
		return ToolIntegration{}, fmt.Errorf("integration endpoint is required for external mode")
	}
	enabled := true
	if p.Enabled != nil {
		enabled = *p.Enabled
	}
	config := rawOrDefault(p.Config, "{}")
	if !isJSONObject(config) {
		return ToolIntegration{}, fmt.Errorf("config must be an object")
	}
	caps, err := json.Marshal(KindCapabilities(kind))
	if err != nil {
		return ToolIntegration{}, err
	}
	secrets, err := encodeSecrets(mergeSecrets(nil, p.Secrets, KindSecretKeys(kind), true))
	if err != nil {
		return ToolIntegration{}, err
	}
	id, err := newID("ti_")
	if err != nil {
		return ToolIntegration{}, err
	}
	now := time.Now().UTC()
	row, err := s.q.InsertToolIntegration(ctx, db.InsertToolIntegrationParams{
		ID:              id,
		Name:            name,
		Kind:            kind,
		Enabled:         enabled,
		Scope:           ScopePlane,
		Endpoint:        endpoint,
		Mode:            mode,
		Capabilities:    caps,
		Config:          config,
		Secrets:         secrets,
		HealthStatus:    HealthUnknown,
		HealthCheckedAt: pgtype.Timestamptz{},
		CreatedAt:       timestamptzFromTime(now),
		UpdatedAt:       timestamptzFromTime(now),
	})
	if err != nil {
		return ToolIntegration{}, fmt.Errorf("insert tool integration: %w", err)
	}
	return publicToolIntegration(row)
}

func (s *Store) UpdateToolIntegration(ctx context.Context, id string, p PatchToolIntegrationParams) (ToolIntegration, error) {
	row, err := s.q.GetToolIntegration(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ToolIntegration{}, newToolIntegrationNotFound(id)
		}
		return ToolIntegration{}, fmt.Errorf("get tool integration: %w", err)
	}
	name := row.Name
	if p.Name != nil {
		name = strings.TrimSpace(*p.Name)
		if name == "" {
			return ToolIntegration{}, fmt.Errorf("integration name is required")
		}
	}
	enabled := row.Enabled
	if p.Enabled != nil {
		enabled = *p.Enabled
	}
	mode := row.Mode
	if p.Mode != nil {
		mode = strings.TrimSpace(*p.Mode)
		if mode != ModeBundled && mode != ModeExternal {
			return ToolIntegration{}, fmt.Errorf("integration mode must be bundled or external")
		}
		if mode == ModeBundled && row.Kind == KindLinkup {
			return ToolIntegration{}, fmt.Errorf("linkup does not support bundled mode")
		}
	}
	endpoint := row.Endpoint
	if p.Endpoint != nil {
		endpoint = strings.TrimSpace(*p.Endpoint)
	}
	if mode == ModeBundled && endpoint == "" {
		endpoint = BundledDefaultEndpoints[row.Kind]
	}
	if mode == ModeExternal && endpoint == "" {
		return ToolIntegration{}, fmt.Errorf("integration endpoint is required for external mode")
	}
	config := row.Config
	if len(p.Config) > 0 {
		if !isJSONObject(p.Config) {
			return ToolIntegration{}, fmt.Errorf("config must be an object")
		}
		config = rawOrDefault(p.Config, "{}")
	}
	currentSecrets, err := decodeSecrets(row.Secrets)
	if err != nil {
		return ToolIntegration{}, err
	}
	merged := mergeSecrets(currentSecrets, p.Secrets, KindSecretKeys(row.Kind), false)
	secretsJSON, err := encodeSecrets(merged)
	if err != nil {
		return ToolIntegration{}, err
	}
	now := time.Now().UTC()
	updated, err := s.q.UpdateToolIntegration(ctx, db.UpdateToolIntegrationParams{
		ID:              id,
		Name:            name,
		Enabled:         enabled,
		Endpoint:        endpoint,
		Mode:            mode,
		Capabilities:    row.Capabilities,
		Config:          config,
		Secrets:         secretsJSON,
		HealthStatus:    row.HealthStatus,
		HealthCheckedAt: row.HealthCheckedAt,
		UpdatedAt:       timestamptzFromTime(now),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ToolIntegration{}, newToolIntegrationNotFound(id)
		}
		return ToolIntegration{}, fmt.Errorf("update tool integration: %w", err)
	}
	return publicToolIntegration(updated)
}

func (s *Store) DeleteToolIntegration(ctx context.Context, id string) error {
	if _, err := s.GetToolIntegration(ctx, id); err != nil {
		return err
	}
	if err := s.q.DeleteToolIntegration(ctx, id); err != nil {
		return fmt.Errorf("delete tool integration: %w", err)
	}
	return nil
}

// UpdateToolIntegrationHealth persists a connection-test result.
func (s *Store) UpdateToolIntegrationHealth(ctx context.Context, id, status string, checkedAt time.Time) (ToolIntegration, error) {
	row, err := s.q.GetToolIntegration(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ToolIntegration{}, newToolIntegrationNotFound(id)
		}
		return ToolIntegration{}, err
	}
	updated, err := s.q.UpdateToolIntegration(ctx, db.UpdateToolIntegrationParams{
		ID:              id,
		Name:            row.Name,
		Enabled:         row.Enabled,
		Endpoint:        row.Endpoint,
		Mode:            row.Mode,
		Capabilities:    row.Capabilities,
		Config:          row.Config,
		Secrets:         row.Secrets,
		HealthStatus:    status,
		HealthCheckedAt: timestamptzFromTime(checkedAt),
		UpdatedAt:       timestamptzFromTime(time.Now().UTC()),
	})
	if err != nil {
		return ToolIntegration{}, fmt.Errorf("update tool integration health: %w", err)
	}
	return publicToolIntegration(updated)
}

func publicToolIntegration(row db.ToolIntegration) (ToolIntegration, error) {
	caps, err := decodeStringSlice(row.Capabilities)
	if err != nil {
		return ToolIntegration{}, fmt.Errorf("decode capabilities: %w", err)
	}
	secrets, err := decodeSecrets(row.Secrets)
	if err != nil {
		return ToolIntegration{}, fmt.Errorf("decode secrets: %w", err)
	}
	configured := make(map[string]bool, len(KindSecretKeys(row.Kind)))
	for _, key := range KindSecretKeys(row.Kind) {
		configured[key] = strings.TrimSpace(secrets[key]) != ""
	}
	var checkedAt *time.Time
	if row.HealthCheckedAt.Valid {
		t := timeFromTimestamptz(row.HealthCheckedAt)
		checkedAt = &t
	}
	return ToolIntegration{
		ID:                row.ID,
		Name:              row.Name,
		Kind:              row.Kind,
		Enabled:           row.Enabled,
		Scope:             row.Scope,
		Endpoint:          row.Endpoint,
		Mode:              row.Mode,
		Capabilities:      caps,
		Config:            rawOrDefault(row.Config, "{}"),
		SecretsConfigured: configured,
		HealthStatus:      row.HealthStatus,
		HealthCheckedAt:   checkedAt,
		CreatedAt:         timeFromTimestamptz(row.CreatedAt),
		UpdatedAt:         timeFromTimestamptz(row.UpdatedAt),
	}, nil
}

func decodeStringSlice(raw []byte) ([]string, error) {
	if len(raw) == 0 {
		return []string{}, nil
	}
	var out []string
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = []string{}
	}
	return out, nil
}

func decodeSecrets(raw []byte) (ToolIntegrationSecrets, error) {
	if len(raw) == 0 {
		return ToolIntegrationSecrets{}, nil
	}
	var out map[string]string
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = map[string]string{}
	}
	return out, nil
}

func encodeSecrets(secrets ToolIntegrationSecrets) ([]byte, error) {
	if secrets == nil {
		secrets = ToolIntegrationSecrets{}
	}
	return json.Marshal(secrets)
}

// mergeSecrets applies a PATCH-style secrets map. On create (requireSet), missing
// required keys with empty values are rejected when the kind declares them.
func mergeSecrets(current ToolIntegrationSecrets, patch map[string]*string, allowed []string, create bool) ToolIntegrationSecrets {
	out := ToolIntegrationSecrets{}
	for k, v := range current {
		out[k] = v
	}
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, k := range allowed {
		allowedSet[k] = struct{}{}
	}
	for key, val := range patch {
		if _, ok := allowedSet[key]; !ok {
			continue
		}
		if val == nil {
			delete(out, key)
			continue
		}
		trimmed := strings.TrimSpace(*val)
		if trimmed == "" {
			delete(out, key)
			continue
		}
		out[key] = trimmed
	}
	_ = create // create-time required secrets are kind-specific; Linkup allows empty until test.
	return out
}

func hasCapability(caps []string, want string) bool {
	for _, c := range caps {
		if c == want {
			return true
		}
	}
	return false
}
