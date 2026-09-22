package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/tryy3/agent-fabric/internal/db"
	"github.com/tryy3/agent-fabric/internal/sandbox"
)

var (
	ErrResourceNotFound = errors.New("resource not found")
	ErrResourceInUse    = errors.New("resource in use")
	KindContainer       = "container"
)

var volumeIDPattern = regexp.MustCompile(`^vol_[0-9a-f]{16}$`)

type Resource struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Kind      string          `json:"kind"`
	Spec      json.RawMessage `json:"spec"`
	CreatedAt time.Time       `json:"createdAt"`
	UpdatedAt time.Time       `json:"updatedAt"`
}

type containerSpec struct {
	Image          string       `json:"image"`
	Dockerfile     string       `json:"dockerfile,omitempty"`
	BuildContext   string       `json:"buildContext,omitempty"`
	ContainerName  string       `json:"containerName"`
	IdleTTLSeconds int64        `json:"idleTTLSeconds"`
	Volumes        []volumeSpec `json:"volumes"`
}

type volumeSpec struct {
	ID          string `json:"id"`
	Enabled     bool   `json:"enabled"`
	Name        string `json:"name"`
	Target      string `json:"target"`
	Whitelisted bool   `json:"whitelisted"`
	Read        bool   `json:"read"`
	Write       bool   `json:"write"`
	Exec        bool   `json:"exec"`
}

type resourceNotFoundError struct {
	id string
}

func (e resourceNotFoundError) Error() string {
	return fmt.Sprintf("resource %q not found", e.id)
}

func (e resourceNotFoundError) Is(target error) bool {
	return target == ErrResourceNotFound
}

func newResourceNotFound(id string) error {
	return resourceNotFoundError{id: id}
}

func (s *Store) ListResources(ctx context.Context) ([]Resource, error) {
	rows, err := s.q.ListResources(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Resource, 0, len(rows))
	for _, row := range rows {
		out = append(out, resourceFromDB(row))
	}
	return out, nil
}

func (s *Store) GetResource(ctx context.Context, id string) (Resource, error) {
	row, err := s.q.GetResource(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Resource{}, newResourceNotFound(id)
		}
		return Resource{}, err
	}
	return resourceFromDB(row), nil
}

func (s *Store) CreateResource(ctx context.Context, name, kind string, spec json.RawMessage) (Resource, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Resource{}, fmt.Errorf("resource name is required")
	}
	normalized, err := s.normalizeContainerSpec(ctx, kind, spec, "")
	if err != nil {
		return Resource{}, err
	}

	id, err := newID("res_")
	if err != nil {
		return Resource{}, err
	}
	now := time.Now().UTC()
	row, err := s.q.InsertResource(ctx, db.InsertResourceParams{
		ID:        id,
		Name:      name,
		Kind:      kind,
		Spec:      normalized,
		CreatedAt: timestamptzFromTime(now),
		UpdatedAt: timestamptzFromTime(now),
	})
	if err != nil {
		return Resource{}, err
	}
	return resourceFromDB(row), nil
}

func (s *Store) UpdateResource(ctx context.Context, id string, name *string, spec json.RawMessage) (Resource, error) {
	current, err := s.GetResource(ctx, id)
	if err != nil {
		return Resource{}, err
	}

	nextName := current.Name
	if name != nil {
		trimmed := strings.TrimSpace(*name)
		if trimmed == "" {
			return Resource{}, fmt.Errorf("resource name is required")
		}
		nextName = trimmed
	}

	nextSpec := current.Spec
	if len(spec) > 0 {
		merged, err := PatchOverlayJSON(current.Spec, spec)
		if err != nil {
			return Resource{}, err
		}
		nextSpec = merged
	}

	normalized, err := s.normalizeContainerSpec(ctx, current.Kind, nextSpec, id)
	if err != nil {
		return Resource{}, err
	}

	now := time.Now().UTC()
	row, err := s.q.UpdateResource(ctx, db.UpdateResourceParams{
		ID:        id,
		Name:      nextName,
		Spec:      normalized,
		UpdatedAt: timestamptzFromTime(now),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Resource{}, newResourceNotFound(id)
		}
		return Resource{}, err
	}
	return resourceFromDB(row), nil
}

func (s *Store) DeleteResource(ctx context.Context, id string) error {
	if _, err := s.GetResource(ctx, id); err != nil {
		return err
	}
	return s.q.DeleteResource(ctx, id)
}

func resourceFromDB(row db.Resource) Resource {
	return Resource{
		ID:        row.ID,
		Name:      row.Name,
		Kind:      row.Kind,
		Spec:      rawOrDefault(row.Spec, "{}"),
		CreatedAt: timeFromTimestamptz(row.CreatedAt),
		UpdatedAt: timeFromTimestamptz(row.UpdatedAt),
	}
}

func (s *Store) normalizeContainerSpec(ctx context.Context, kind string, raw json.RawMessage, excludeID string) (json.RawMessage, error) {
	if kind != KindContainer {
		return nil, fmt.Errorf("resource kind must be %q", KindContainer)
	}

	specMap := map[string]json.RawMessage{}
	if len(raw) > 0 && !isJSONNull(raw) {
		if err := json.Unmarshal(raw, &specMap); err != nil {
			return nil, fmt.Errorf("decode resource spec: %w", err)
		}
	}
	idlePresent := false
	if _, ok := specMap["idleTTLSeconds"]; ok {
		idlePresent = true
	}

	var spec containerSpec
	if err := json.Unmarshal(raw, &spec); err != nil {
		return nil, fmt.Errorf("decode resource spec: %w", err)
	}

	spec.Image = strings.TrimSpace(spec.Image)
	spec.Dockerfile = strings.TrimSpace(spec.Dockerfile)
	spec.BuildContext = strings.TrimSpace(spec.BuildContext)
	spec.ContainerName = strings.TrimSpace(spec.ContainerName)

	if spec.Image == "" {
		return nil, fmt.Errorf("container image is required")
	}
	if spec.ContainerName == "" {
		return nil, fmt.Errorf("container name is required")
	}
	if idlePresent && spec.IdleTTLSeconds <= 0 {
		return nil, fmt.Errorf("idleTTLSeconds must be a positive integer")
	}
	if !idlePresent {
		spec.IdleTTLSeconds = DefaultIdleTTLSeconds
	}

	spec.ContainerName = ApplyIdentityPrefix(spec.ContainerName, s.IdentityPrefix)
	if err := sandbox.ValidateContainerName(spec.ContainerName); err != nil {
		return nil, err
	}

	targets := map[string]struct{}{}
	volumeNames := map[string]struct{}{}
	for i := range spec.Volumes {
		v := &spec.Volumes[i]
		v.ID = strings.TrimSpace(v.ID)
		v.Name = strings.TrimSpace(v.Name)
		v.Target = strings.TrimSpace(v.Target)
		if !volumeIDPattern.MatchString(v.ID) {
			return nil, fmt.Errorf("volume id %q is invalid", v.ID)
		}
		if v.Name == "" || v.Target == "" {
			return nil, fmt.Errorf("volume name and target are required")
		}
		v.Name = ApplyIdentityPrefix(v.Name, s.IdentityPrefix)
		if err := sandbox.ValidateVolumeName(v.Name); err != nil {
			return nil, err
		}
		if _, ok := targets[v.Target]; ok {
			return nil, fmt.Errorf("duplicate mount target %q", v.Target)
		}
		targets[v.Target] = struct{}{}
		if _, ok := volumeNames[v.Name]; ok {
			return nil, fmt.Errorf("duplicate volume name %q", v.Name)
		}
		volumeNames[v.Name] = struct{}{}
	}

	if err := s.rejectDuplicateResourceNames(ctx, excludeID, spec.ContainerName, volumeNames); err != nil {
		return nil, err
	}

	out, err := json.Marshal(spec)
	if err != nil {
		return nil, fmt.Errorf("encode resource spec: %w", err)
	}
	return out, nil
}

func (s *Store) rejectDuplicateResourceNames(ctx context.Context, excludeID, containerName string, volumeNames map[string]struct{}) error {
	rows, err := s.q.ListResources(ctx)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if row.ID == excludeID {
			continue
		}
		if row.Kind != KindContainer {
			continue
		}
		other, err := decodeContainerSpec(row.Spec)
		if err != nil {
			return err
		}
		if other.ContainerName == containerName {
			return fmt.Errorf("duplicate container name %q", containerName)
		}
		for _, vol := range other.Volumes {
			name := strings.TrimSpace(vol.Name)
			if name == "" {
				continue
			}
			if _, ok := volumeNames[name]; ok {
				return fmt.Errorf("duplicate volume name %q", name)
			}
		}
	}
	return nil
}

func decodeContainerSpec(raw []byte) (containerSpec, error) {
	var spec containerSpec
	if len(raw) == 0 {
		return spec, nil
	}
	if err := json.Unmarshal(raw, &spec); err != nil {
		return containerSpec{}, fmt.Errorf("decode resource spec: %w", err)
	}
	return spec, nil
}
