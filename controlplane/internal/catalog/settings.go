package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/tryy3/agent-fabric/internal/db"
)

type DeprecatedSandbox struct {
	Kind           string
	WorkspaceRoot  string
	Image          string
	Dockerfile     string
	BuildContext   string
	IdleTTLSeconds int64
}

func (d DeprecatedSandbox) HasKeys() bool {
	return d.Kind != "" || d.WorkspaceRoot != "" || d.Image != "" ||
		d.Dockerfile != "" || d.BuildContext != "" || d.IdleTTLSeconds != 0
}

func (s *Store) GetPlaneSettings(ctx context.Context) (PlaneSettings, error) {
	settings, _, err := s.ensurePlaneSettings(ctx, DeprecatedSandbox{})
	return settings, err
}

func (s *Store) EnsurePlaneSettings(ctx context.Context, deprecated DeprecatedSandbox) (PlaneSettings, error) {
	settings, _, err := s.ensurePlaneSettings(ctx, deprecated)
	return settings, err
}

func (s *Store) PatchPlaneSettings(ctx context.Context, sandboxPatch json.RawMessage) (PlaneSettings, error) {
	if len(sandboxPatch) == 0 {
		return PlaneSettings{}, fmt.Errorf("sandbox patch is required")
	}
	current, err := s.GetPlaneSettings(ctx)
	if err != nil {
		return PlaneSettings{}, err
	}
	patched, err := PatchOverlayJSON(current.Sandbox, sandboxPatch)
	if err != nil {
		return PlaneSettings{}, err
	}
	now := time.Now().UTC()
	row, err := s.q.UpdatePlaneSettings(ctx, db.UpdatePlaneSettingsParams{
		Sandbox:   patched,
		UpdatedAt: timestamptzFromTime(now),
	})
	if err != nil {
		return PlaneSettings{}, fmt.Errorf("update plane settings: %w", err)
	}
	return planeSettingsFromDB(row), nil
}

func (s *Store) ensurePlaneSettings(ctx context.Context, deprecated DeprecatedSandbox) (PlaneSettings, bool, error) {
	row, err := s.q.GetPlaneSettings(ctx)
	if err == nil {
		return planeSettingsFromDB(row), false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return PlaneSettings{}, false, fmt.Errorf("get plane settings: %w", err)
	}

	preservePhase1, err := s.preservePhase1Volumes(ctx)
	if err != nil {
		return PlaneSettings{}, false, err
	}
	overlay := DefaultOverlay(preservePhase1)
	if deprecated.HasKeys() {
		overlay = applyDeprecated(overlay, deprecated)
	}
	raw, err := EncodeOverlay(overlay)
	if err != nil {
		return PlaneSettings{}, false, err
	}
	now := time.Now().UTC()
	inserted, err := s.q.InsertPlaneSettings(ctx, db.InsertPlaneSettingsParams{
		Sandbox:   raw,
		CreatedAt: timestamptzFromTime(now),
		UpdatedAt: timestamptzFromTime(now),
	})
	if err != nil {
		existing, getErr := s.q.GetPlaneSettings(ctx)
		if getErr == nil {
			return planeSettingsFromDB(existing), false, nil
		}
		return PlaneSettings{}, false, fmt.Errorf("insert plane settings: %w", err)
	}
	return planeSettingsFromDB(inserted), true, nil
}

func (s *Store) preservePhase1Volumes(ctx context.Context) (bool, error) {
	threads, err := s.q.CountAllThreads(ctx)
	if err != nil {
		return false, fmt.Errorf("count threads: %w", err)
	}
	if threads > 0 {
		return true, nil
	}
	extra, err := s.q.CountNonPersonalProjects(ctx)
	if err != nil {
		return false, fmt.Errorf("count projects: %w", err)
	}
	return extra > 0, nil
}

func applyDeprecated(overlay Overlay, deprecated DeprecatedSandbox) Overlay {
	if deprecated.Kind != "" {
		kind := deprecated.Kind
		overlay.Kind = &kind
	}
	if deprecated.WorkspaceRoot != "" && deprecated.Kind != "local" {
		root := deprecated.WorkspaceRoot
		overlay.WorkspaceRoot = &root
	}
	if deprecated.Image != "" {
		image := deprecated.Image
		overlay.Image = &image
	}
	if deprecated.Dockerfile != "" {
		dockerfile := deprecated.Dockerfile
		overlay.Dockerfile = &dockerfile
	}
	if deprecated.BuildContext != "" {
		buildContext := deprecated.BuildContext
		overlay.BuildContext = &buildContext
	}
	if deprecated.IdleTTLSeconds != 0 {
		ttl := deprecated.IdleTTLSeconds
		overlay.IdleTTLSeconds = &ttl
	}
	return overlay
}

func planeSettingsFromDB(row db.PlaneSetting) PlaneSettings {
	return PlaneSettings{Sandbox: rawOrDefault(row.Sandbox, "{}")}
}
