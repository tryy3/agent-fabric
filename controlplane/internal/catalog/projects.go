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

func (s *Store) CreateProject(ctx context.Context, name, description, isolation string) (Project, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Project{}, fmt.Errorf("project name is required")
	}
	isolation, err := normalizeIsolation(isolation)
	if err != nil {
		return Project{}, err
	}
	if isolation == IsolationShared {
		return Project{}, fmt.Errorf("shared isolation requires environmentId")
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
		Isolation:   isolation,
		Settings:    []byte("{}"),
		Remotes:     []byte("[]"),
		CreatedAt:   timestamptzFromTime(now),
		UpdatedAt:   timestamptzFromTime(now),
	})
	if err != nil {
		return Project{}, fmt.Errorf("create project: %w", err)
	}
	return projectFromDB(row), nil
}

func (s *Store) CreateSharedProject(ctx context.Context, name, description, environmentID string) (Project, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Project{}, fmt.Errorf("project name is required")
	}
	environmentID = strings.TrimSpace(environmentID)
	if environmentID == "" {
		return Project{}, fmt.Errorf("shared isolation requires environmentId")
	}
	if _, err := s.GetEnvironment(ctx, environmentID); err != nil {
		return Project{}, err
	}

	id, err := newID("proj_")
	if err != nil {
		return Project{}, err
	}
	now := time.Now().UTC()
	row, err := s.q.InsertProject(ctx, db.InsertProjectParams{
		ID:            id,
		Name:          name,
		Description:   description,
		Isolation:     IsolationShared,
		EnvironmentID: &environmentID,
		Settings:      []byte("{}"),
		Remotes:       []byte("[]"),
		CreatedAt:     timestamptzFromTime(now),
		UpdatedAt:     timestamptzFromTime(now),
	})
	if err != nil {
		return Project{}, fmt.Errorf("create project: %w", err)
	}
	return projectFromDB(row), nil
}

func (s *Store) GetEnvironment(ctx context.Context, id string) (Environment, error) {
	row, err := s.q.GetEnvironment(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Environment{}, newEnvironmentNotFound(id)
		}
		return Environment{}, fmt.Errorf("get environment: %w", err)
	}
	return environmentFromDB(row), nil
}

func (s *Store) CreateEnvironment(ctx context.Context, name, kind string, volumeName *string) (Environment, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Environment{}, fmt.Errorf("environment name is required")
	}
	kind = strings.TrimSpace(kind)
	if kind == "" {
		kind = "docker"
	}
	if kind != "local" && kind != "docker" {
		return Environment{}, fmt.Errorf("unknown environment kind %q", kind)
	}
	id, err := newID("env_")
	if err != nil {
		return Environment{}, err
	}
	now := time.Now().UTC()
	row, err := s.q.InsertEnvironment(ctx, db.InsertEnvironmentParams{
		ID:         id,
		Name:       name,
		Kind:       kind,
		Spec:       []byte("{}"),
		VolumeName: volumeName,
		CreatedAt:  timestamptzFromTime(now),
		UpdatedAt:  timestamptzFromTime(now),
	})
	if err != nil {
		return Environment{}, fmt.Errorf("create environment: %w", err)
	}
	return environmentFromDB(row), nil
}

func (s *Store) UpdateProject(ctx context.Context, id string, name, description, isolation *string, settings json.RawMessage, remotes ...json.RawMessage) (Project, error) {
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
	if isolation != nil {
		iso, err := normalizeIsolation(*isolation)
		if err != nil {
			return Project{}, err
		}
		if iso == IsolationShared && current.EnvironmentID == nil {
			return Project{}, fmt.Errorf("shared isolation requires environmentId")
		}
		current.Isolation = iso
		if iso == IsolationIsolated {
			current.EnvironmentID = nil
		}
	}
	if len(settings) > 0 {
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
		ID:            id,
		Name:          current.Name,
		Description:   current.Description,
		Isolation:     current.Isolation,
		EnvironmentID: current.EnvironmentID,
		Settings:      rawOrDefault(current.Settings, "{}"),
		Remotes:       rawOrDefault(current.Remotes, "[]"),
		UpdatedAt:     timestamptzFromTime(now),
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
			created, err := s.CreateProject(ctx, DefaultProjectName, "", IsolationIsolated)
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
		ID:            row.ID,
		Name:          row.Name,
		Description:   row.Description,
		Isolation:     row.Isolation,
		EnvironmentID: row.EnvironmentID,
		Settings:      rawOrDefault(row.Settings, "{}"),
		Remotes:       rawOrDefault(row.Remotes, "[]"),
		CreatedAt:     timeFromTimestamptz(row.CreatedAt),
		UpdatedAt:     timeFromTimestamptz(row.UpdatedAt),
	}
}

func environmentFromDB(row db.Environment) Environment {
	return Environment{
		ID:         row.ID,
		Name:       row.Name,
		Kind:       row.Kind,
		Spec:       rawOrDefault(row.Spec, "{}"),
		VolumeName: row.VolumeName,
		CreatedAt:  timeFromTimestamptz(row.CreatedAt),
		UpdatedAt:  timeFromTimestamptz(row.UpdatedAt),
	}
}

func normalizeIsolation(isolation string) (string, error) {
	isolation = strings.TrimSpace(isolation)
	if isolation == "" {
		return IsolationIsolated, nil
	}
	if isolation != IsolationIsolated && isolation != IsolationShared {
		return "", fmt.Errorf("unknown isolation %q", isolation)
	}
	return isolation, nil
}

func rawOrDefault(raw json.RawMessage, fallback string) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage(fallback)
	}
	return raw
}
