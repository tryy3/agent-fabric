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

func (s *Store) ListProjects(ctx context.Context) ([]Project, error) {
	rows, err := s.q.ListProjects(ctx)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	out := make([]Project, 0, len(rows))
	for _, row := range rows {
		out = append(out, projectFromDB(row))
	}
	return out, nil
}

func (s *Store) GetProject(ctx context.Context, id string) (Project, error) {
	row, err := s.q.GetProject(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Project{}, newProjectNotFound(id)
		}
		return Project{}, fmt.Errorf("get project: %w", err)
	}
	return projectFromDB(row), nil
}

func (s *Store) CreateProject(ctx context.Context, name, description string) (Project, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Project{}, fmt.Errorf("project name is required")
	}

	id, err := newID("proj_")
	if err != nil {
		return Project{}, err
	}
	now := time.Now().UTC()
	row, err := s.q.InsertProject(ctx, db.InsertProjectParams{
		ID:          id,
		Name:        name,
		Description: description,
		Settings:    []byte("{}"),
		Remotes:     []byte("[]"),
		CreatedAt:   timestamptzFromTime(now),
		UpdatedAt:   timestamptzFromTime(now),
	})
	if err != nil {
		return Project{}, fmt.Errorf("create project: %w", err)
	}
	project := projectFromDB(row)

	linked, err := s.linkIsolatedResource(ctx, project)
	if err != nil {
		if delErr := s.q.DeleteProject(ctx, project.ID); delErr != nil {
			return Project{}, fmt.Errorf("provision project resource: %w (cleanup: %v)", err, delErr)
		}
		return Project{}, fmt.Errorf("provision project resource: %w", err)
	}
	return linked, nil
}

// linkIsolatedResource creates a dedicated container resource for the project
// and stores settings.environment.resourceId, matching backfill's isolated shape.
func (s *Store) linkIsolatedResource(ctx context.Context, project Project) (Project, error) {
	containerName, err := ExpandName(DefaultContainerNameTemplate, NameVars{ProjectID: project.ID})
	if err != nil {
		return Project{}, err
	}
	volumeName, err := ExpandName(DefaultVolumeNameTemplate, NameVars{ProjectID: project.ID})
	if err != nil {
		return Project{}, err
	}
	volumeID, err := newID("vol_")
	if err != nil {
		return Project{}, err
	}
	spec, err := json.Marshal(containerSpec{
		Image:          DefaultSandboxImage,
		ContainerName:  containerName,
		IdleTTLSeconds: DefaultIdleTTLSeconds,
		Volumes: []volumeSpec{{
			ID:          volumeID,
			Enabled:     true,
			Name:        volumeName,
			Target:      DefaultWorkspaceRoot,
			Whitelisted: true,
			Read:        true,
			Write:       true,
			Exec:        true,
		}},
	})
	if err != nil {
		return Project{}, fmt.Errorf("encode container spec: %w", err)
	}
	resource, err := s.CreateResource(ctx, project.Name, KindContainer, spec)
	if err != nil {
		return Project{}, err
	}
	env, err := json.Marshal(map[string]any{
		"environment": map[string]string{"resourceId": resource.ID},
	})
	if err != nil {
		return Project{}, fmt.Errorf("encode environment: %w", err)
	}
	return s.UpdateProject(ctx, project.ID, nil, nil, env)
}

func (s *Store) UpdateProject(ctx context.Context, id string, name, description *string, settings json.RawMessage, remotes ...json.RawMessage) (Project, error) {
	current, err := s.GetProject(ctx, id)
	if err != nil {
		return Project{}, err
	}
	if name != nil {
		trimmed := strings.TrimSpace(*name)
		if trimmed == "" {
			return Project{}, fmt.Errorf("project name is required")
		}
		if current.Name == DefaultProjectName && trimmed != DefaultProjectName {
			return Project{}, ErrDefaultProjectRename
		}
		current.Name = trimmed
	}
	if description != nil {
		current.Description = *description
	}
	if len(settings) > 0 {
		if patchHasEnvironment(settings) {
			var patchBag map[string]json.RawMessage
			if err := json.Unmarshal(settings, &patchBag); err != nil {
				return Project{}, fmt.Errorf("decode settings patch: %w", err)
			}
			globalSettings, err := s.GetPlaneSettings(ctx)
			if err != nil {
				return Project{}, err
			}
			storedEnv, err := EnvironmentFromSettings(current.Settings)
			if err != nil {
				return Project{}, err
			}
			if err := s.validateEnvironmentPatch(ctx, patchBag["environment"], storedEnv, globalSettings.Environment); err != nil {
				return Project{}, err
			}
		}
		merged, err := MergeSettings(current.Settings, settings)
		if err != nil {
			return Project{}, err
		}
		current.Settings = merged
	}
	if len(remotes) > 0 && len(remotes[0]) > 0 {
		patched, err := PatchRemotesJSON(current.Remotes, remotes[0])
		if err != nil {
			return Project{}, err
		}
		current.Remotes = patched
	}

	now := time.Now().UTC()
	row, err := s.q.UpdateProject(ctx, db.UpdateProjectParams{
		ID:          id,
		Name:        current.Name,
		Description: current.Description,
		Settings:    rawOrDefault(current.Settings, "{}"),
		Remotes:     rawOrDefault(current.Remotes, "[]"),
		UpdatedAt:   timestamptzFromTime(now),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Project{}, newProjectNotFound(id)
		}
		return Project{}, fmt.Errorf("update project: %w", err)
	}
	return projectFromDB(row), nil
}

func (s *Store) DeleteProject(ctx context.Context, id string) error {
	return s.inTx(ctx, func(q *db.Queries) error {
		row, err := q.GetProject(ctx, id)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return newProjectNotFound(id)
			}
			return fmt.Errorf("get project: %w", err)
		}
		if row.Name == DefaultProjectName {
			return ErrDefaultProject
		}
		if err := q.DeleteThreadsByProject(ctx, id); err != nil {
			return fmt.Errorf("delete project threads: %w", err)
		}
		if err := q.DeleteProject(ctx, id); err != nil {
			if isFKViolation(err) {
				return ErrProjectInUse
			}
			return fmt.Errorf("delete project: %w", err)
		}
		return nil
	})
}

func (s *Store) resolveProjectID(ctx context.Context, projectID string) (string, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID != "" {
		if _, err := s.GetProject(ctx, projectID); err != nil {
			return "", err
		}
		return projectID, nil
	}
	row, err := s.q.GetDefaultProject(ctx)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			created, err := s.CreateProject(ctx, DefaultProjectName, "")
			if err != nil {
				return "", err
			}
			return created.ID, nil
		}
		return "", fmt.Errorf("get default project: %w", err)
	}
	return row.ID, nil
}

func projectFromDB(row db.Project) Project {
	return Project{
		ID:          row.ID,
		Name:        row.Name,
		Description: row.Description,
		Settings:    rawOrDefault(row.Settings, "{}"),
		Remotes:     rawOrDefault(row.Remotes, "[]"),
		CreatedAt:   timeFromTimestamptz(row.CreatedAt),
		UpdatedAt:   timeFromTimestamptz(row.UpdatedAt),
	}
}

func rawOrDefault(raw json.RawMessage, fallback string) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage(fallback)
	}
	return raw
}
