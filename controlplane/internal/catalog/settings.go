package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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

func (s *Store) PatchPlaneSettings(ctx context.Context, sandboxPatch, environmentPatch json.RawMessage, integrationsPatch ...json.RawMessage) (PlaneSettings, error) {
	var integPatch json.RawMessage
	if len(integrationsPatch) > 0 {
		integPatch = integrationsPatch[0]
	}
	if len(sandboxPatch) == 0 && len(environmentPatch) == 0 && len(integPatch) == 0 {
		return PlaneSettings{}, fmt.Errorf("settings patch is required")
	}
	current, err := s.GetPlaneSettings(ctx)
	if err != nil {
		return PlaneSettings{}, err
	}
	nextSandbox := current.Sandbox
	if len(sandboxPatch) > 0 {
		patched, patchErr := PatchOverlayJSON(current.Sandbox, sandboxPatch)
		if patchErr != nil {
			return PlaneSettings{}, patchErr
		}
		nextSandbox = patched
	}
	nextEnvironment := current.Environment
	if len(environmentPatch) > 0 {
		if err := s.validateEnvironmentPatch(ctx, environmentPatch, current.Environment, current.Environment); err != nil {
			return PlaneSettings{}, err
		}
		patched, patchErr := PatchEnvironmentJSON(current.Environment, environmentPatch)
		if patchErr != nil {
			return PlaneSettings{}, patchErr
		}
		nextEnvironment = patched
	}
	nextIntegrations := current.Integrations
	if len(integPatch) > 0 {
		patched, patchErr := PatchIntegrationsJSON(current.Integrations, integPatch)
		if patchErr != nil {
			return PlaneSettings{}, patchErr
		}
		nextIntegrations = patched
	}
	now := time.Now().UTC()
	if len(sandboxPatch) > 0 || len(environmentPatch) > 0 {
		_, err := s.q.UpdatePlaneSettings(ctx, db.UpdatePlaneSettingsParams{
			Sandbox:     nextSandbox,
			Environment: nextEnvironment,
			UpdatedAt:   timestamptzFromTime(now),
		})
		if err != nil {
			return PlaneSettings{}, fmt.Errorf("update plane settings: %w", err)
		}
	}
	if len(integPatch) > 0 {
		if err := s.q.UpdatePlaneIntegrations(ctx, db.UpdatePlaneIntegrationsParams{
			Integrations: nextIntegrations,
			UpdatedAt:    timestamptzFromTime(now),
		}); err != nil {
			return PlaneSettings{}, fmt.Errorf("update plane integrations: %w", err)
		}
	}
	return PlaneSettings{
		Sandbox:      rawOrDefault(nextSandbox, "{}"),
		Environment:  rawOrDefault(nextEnvironment, "{}"),
		Integrations: rawOrDefault(nextIntegrations, "{}"),
	}, nil
}

func (s *Store) ensurePlaneSettings(ctx context.Context, deprecated DeprecatedSandbox) (PlaneSettings, bool, error) {
	row, err := s.q.GetPlaneSettings(ctx)
	if err == nil {
		return s.planeSettingsFromCore(ctx, row.Sandbox, row.Environment), false, nil
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
			return s.planeSettingsFromCore(ctx, existing.Sandbox, existing.Environment), false, nil
		}
		return PlaneSettings{}, false, fmt.Errorf("insert plane settings: %w", err)
	}
	return s.planeSettingsFromCore(ctx, inserted.Sandbox, inserted.Environment), true, nil
}

func (s *Store) preservePhase1Volumes(ctx context.Context) (bool, error) {
	threads, err := s.q.CountAllThreads(ctx)
	if err != nil {
		return false, fmt.Errorf("count threads: %w", err)
	}
	if threads > 0 {
		return true, nil
	}
	extra, err := s.q.CountNonDefaultProjects(ctx)
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

func (s *Store) planeSettingsFromCore(ctx context.Context, sandbox, environment []byte) PlaneSettings {
	return PlaneSettings{
		Sandbox:      rawOrDefault(sandbox, "{}"),
		Environment:  rawOrDefault(environment, "{}"),
		Integrations: s.loadIntegrations(ctx),
	}
}

// loadIntegrations returns the integrations bag, or {} when the column is not
// migrated yet (appmigrate pauses at v10 before 00012).
func (s *Store) loadIntegrations(ctx context.Context) json.RawMessage {
	if !s.integrationsColumnReady(ctx) {
		return json.RawMessage(`{}`)
	}
	raw, err := s.q.GetPlaneIntegrations(ctx)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return rawOrDefault(raw, "{}")
}

// integrationsColumnReady checks committed schema via the pool (not an ambient
// tx) so a missing column never aborts backfill's transaction.
func (s *Store) integrationsColumnReady(ctx context.Context) bool {
	if s == nil || s.pool == nil {
		return false
	}
	var exists bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = current_schema()
			  AND table_name = 'plane_settings'
			  AND column_name = 'integrations'
		)`).Scan(&exists)
	return err == nil && exists
}

func planeSettingsFromDB(row db.PlaneSetting) PlaneSettings {
	return PlaneSettings{
		Sandbox:      rawOrDefault(row.Sandbox, "{}"),
		Environment:  rawOrDefault(row.Environment, "{}"),
		Integrations: rawOrDefault(row.Integrations, "{}"),
	}
}

func overlayFromSettingsJSON(raw json.RawMessage) (Overlay, error) {
	sandbox, err := SandboxFromSettings(raw)
	if err != nil {
		return Overlay{}, err
	}
	return DecodeOverlay(sandbox)
}

func (s *Store) ResolvedProjectSandbox(ctx context.Context, projectID string) (Overlay, error) {
	project, err := s.GetProject(ctx, projectID)
	if err != nil {
		return Overlay{}, err
	}
	globalSettings, err := s.GetPlaneSettings(ctx)
	if err != nil {
		return Overlay{}, err
	}
	global, err := DecodeOverlay(globalSettings.Sandbox)
	if err != nil {
		return Overlay{}, err
	}
	projectOverlay, err := overlayFromSettingsJSON(project.Settings)
	if err != nil {
		return Overlay{}, err
	}
	resolved := ResolveOverlay(global, projectOverlay)
	return PreviewOverlay(resolved, NameVars{ProjectID: project.ID})
}

func (s *Store) ResolvedAgentSandbox(ctx context.Context, agentID, projectID string) (Overlay, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return Overlay{}, fmt.Errorf("projectId is required")
	}
	agent, err := s.GetAgent(ctx, agentID)
	if err != nil {
		return Overlay{}, err
	}
	project, err := s.GetProject(ctx, projectID)
	if err != nil {
		return Overlay{}, err
	}
	globalSettings, err := s.GetPlaneSettings(ctx)
	if err != nil {
		return Overlay{}, err
	}
	global, err := DecodeOverlay(globalSettings.Sandbox)
	if err != nil {
		return Overlay{}, err
	}
	projectOverlay, err := overlayFromSettingsJSON(project.Settings)
	if err != nil {
		return Overlay{}, err
	}
	agentOverlay, err := overlayFromSettingsJSON(agent.Settings)
	if err != nil {
		return Overlay{}, err
	}
	resolved := ResolveOverlay(global, projectOverlay, agentOverlay)
	return PreviewOverlay(resolved, NameVars{ProjectID: project.ID})
}
