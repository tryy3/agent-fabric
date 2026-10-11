package catalog

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/tryy3/agent-fabric/internal/db"
)

// DefaultPublishListLimit caps how many publishes ListPublishes returns when
// the caller passes no limit.
const DefaultPublishListLimit = 200

// ErrPublishNotFound is returned when a publish id is unknown or belongs to
// another project.
var ErrPublishNotFound = errors.New("publish not found")

// InsertPublish records a successful publish. It becomes the target's live
// deploy; earlier publishes of the same provider target stop being live.
func (s *Store) InsertPublish(ctx context.Context, projectID string, in PublishInput) (Publish, error) {
	if _, err := s.GetProject(ctx, projectID); err != nil {
		return Publish{}, err
	}
	in, err := normalizePublishInput(in)
	if err != nil {
		return Publish{}, err
	}
	id, err := newID("pub_")
	if err != nil {
		return Publish{}, err
	}
	var out Publish
	err = s.inTx(ctx, func(q *db.Queries) error {
		if err := q.ClearProjectPublishLive(ctx, db.ClearProjectPublishLiveParams{
			ProjectID: projectID,
			Provider:  in.Provider,
			Target:    in.Target,
		}); err != nil {
			return fmt.Errorf("clear live publish: %w", err)
		}
		row, err := q.InsertProjectPublish(ctx, insertPublishParams(id, projectID, in, true))
		if err != nil {
			return fmt.Errorf("insert publish: %w", err)
		}
		out = publishFromDB(row)
		return nil
	})
	return out, err
}

// ListPublishes returns a project's publishes, newest first. An empty
// provider lists every provider; limit <= 0 uses DefaultPublishListLimit.
func (s *Store) ListPublishes(ctx context.Context, projectID, provider string, limit int) ([]Publish, error) {
	if _, err := s.GetProject(ctx, projectID); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > DefaultPublishListLimit {
		limit = DefaultPublishListLimit
	}
	rows, err := s.q.ListProjectPublishes(ctx, db.ListProjectPublishesParams{
		ProjectID: projectID,
		Provider:  emptyToNil(&provider),
		RowLimit:  int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("list publishes: %w", err)
	}
	out := make([]Publish, 0, len(rows))
	for _, row := range rows {
		out = append(out, publishFromDB(row))
	}
	return out, nil
}

// GetPublish returns one publish of projectID.
func (s *Store) GetPublish(ctx context.Context, projectID, id string) (Publish, error) {
	row, err := s.q.GetProjectPublish(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Publish{}, fmt.Errorf("%w: %q", ErrPublishNotFound, id)
		}
		return Publish{}, fmt.Errorf("get publish: %w", err)
	}
	if row.ProjectID != projectID {
		return Publish{}, fmt.Errorf("%w: %q", ErrPublishNotFound, id)
	}
	return publishFromDB(row), nil
}

// DeletePublish removes a publish from the history. It does not touch the
// provider; callers delete the deploy there first.
func (s *Store) DeletePublish(ctx context.Context, projectID, id string) error {
	if _, err := s.GetPublish(ctx, projectID, id); err != nil {
		return err
	}
	if err := s.q.DeleteProjectPublish(ctx, id); err != nil {
		return fmt.Errorf("delete publish: %w", err)
	}
	return nil
}

// ReconcilePublishes makes the history of one provider target match the
// deploys the provider reports. Unknown deploys are added, known ones get
// fresh links and live state, and stored deploys the provider no longer has
// are removed. When complete is false the provider's list was truncated, so
// only stored deploys at least as new as the oldest reported one can be
// judged gone; older rows are kept. Rows without a deploy id are never
// removed because they cannot be matched.
func (s *Store) ReconcilePublishes(
	ctx context.Context,
	projectID, provider, target string,
	deploys []PublishInput,
	complete bool,
) (PublishSyncStats, error) {
	var stats PublishSyncStats
	if _, err := s.GetProject(ctx, projectID); err != nil {
		return stats, err
	}
	provider = strings.TrimSpace(provider)
	target = strings.TrimSpace(target)
	if provider == "" {
		return stats, fmt.Errorf("provider is required")
	}
	remote := make(map[string]PublishInput, len(deploys))
	var oldest time.Time
	reportsLive := false
	for _, d := range deploys {
		d.Provider = provider
		d.Target = target
		norm, err := normalizePublishInput(d)
		if err != nil || norm.DeployID == "" {
			continue
		}
		if _, dup := remote[norm.DeployID]; dup {
			continue
		}
		remote[norm.DeployID] = norm
		reportsLive = reportsLive || norm.Live
		if oldest.IsZero() || norm.CreatedAt.Before(oldest) {
			oldest = norm.CreatedAt
		}
	}
	err := s.inTx(ctx, func(q *db.Queries) error {
		rows, err := q.ListProjectPublishesForTarget(ctx, db.ListProjectPublishesForTargetParams{
			ProjectID: projectID,
			Provider:  provider,
			Target:    target,
		})
		if err != nil {
			return fmt.Errorf("list target publishes: %w", err)
		}
		seen := make(map[string]bool, len(rows))
		for _, row := range rows {
			if row.DeployID == "" {
				continue
			}
			seen[row.DeployID] = true
			d, ok := remote[row.DeployID]
			if !ok {
				if !complete && timeFromTimestamptz(row.CreatedAt).Before(oldest) {
					// Outside the reported window: keep it, but only one
					// deploy can be live.
					if row.Live && reportsLive {
						if err := q.UpdateProjectPublishSynced(ctx, db.UpdateProjectPublishSyncedParams{
							ID:        row.ID,
							SiteUrl:   row.SiteUrl,
							DeployUrl: row.DeployUrl,
							Live:      false,
						}); err != nil {
							return fmt.Errorf("update publish: %w", err)
						}
						stats.Updated++
					}
					continue
				}
				if err := q.DeleteProjectPublish(ctx, row.ID); err != nil {
					return fmt.Errorf("remove publish: %w", err)
				}
				stats.Removed++
				continue
			}
			if row.SiteUrl == d.SiteURL && row.DeployUrl == d.DeployURL && row.Live == d.Live {
				continue
			}
			if err := q.UpdateProjectPublishSynced(ctx, db.UpdateProjectPublishSyncedParams{
				ID:        row.ID,
				SiteUrl:   d.SiteURL,
				DeployUrl: d.DeployURL,
				Live:      d.Live,
			}); err != nil {
				return fmt.Errorf("update publish: %w", err)
			}
			stats.Updated++
		}
		for _, d := range deploys {
			norm, ok := remote[strings.TrimSpace(d.DeployID)]
			if !ok || seen[norm.DeployID] {
				continue
			}
			seen[norm.DeployID] = true
			id, err := newID("pub_")
			if err != nil {
				return err
			}
			if _, err := q.InsertProjectPublish(ctx, insertPublishParams(id, projectID, norm, norm.Live)); err != nil {
				return fmt.Errorf("import publish: %w", err)
			}
			stats.Added++
		}
		return nil
	})
	if err != nil {
		return PublishSyncStats{}, err
	}
	return stats, nil
}

func normalizePublishInput(in PublishInput) (PublishInput, error) {
	in.Provider = strings.TrimSpace(in.Provider)
	in.Target = strings.TrimSpace(in.Target)
	in.DeployID = strings.TrimSpace(in.DeployID)
	in.SiteURL = strings.TrimSpace(in.SiteURL)
	in.DeployURL = strings.TrimSpace(in.DeployURL)
	if in.Provider == "" {
		return in, fmt.Errorf("provider is required")
	}
	if in.SiteURL == "" && in.DeployURL == "" {
		return in, fmt.Errorf("site or deploy URL is required")
	}
	if in.CreatedAt.IsZero() {
		in.CreatedAt = time.Now()
	}
	in.CreatedAt = in.CreatedAt.UTC()
	return in, nil
}

func insertPublishParams(id, projectID string, in PublishInput, live bool) db.InsertProjectPublishParams {
	return db.InsertProjectPublishParams{
		ID:        id,
		ProjectID: projectID,
		Provider:  in.Provider,
		Target:    in.Target,
		DeployID:  in.DeployID,
		SiteUrl:   in.SiteURL,
		DeployUrl: in.DeployURL,
		CreatedAt: timestamptzFromTime(in.CreatedAt),
		Live:      live,
	}
}

func publishFromDB(row db.ProjectPublish) Publish {
	return Publish{
		ID:        row.ID,
		ProjectID: row.ProjectID,
		Provider:  row.Provider,
		Target:    row.Target,
		DeployID:  row.DeployID,
		SiteURL:   row.SiteUrl,
		DeployURL: row.DeployUrl,
		Live:      row.Live,
		CreatedAt: timeFromTimestamptz(row.CreatedAt),
	}
}
