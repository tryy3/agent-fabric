package catalog

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tryy3/agent-fabric/internal/db"
)

// DefaultPublishListLimit caps how many publishes ListPublishes returns when
// the caller passes no limit.
const DefaultPublishListLimit = 200

func (s *Store) InsertPublish(ctx context.Context, projectID string, in PublishInput) (Publish, error) {
	if _, err := s.GetProject(ctx, projectID); err != nil {
		return Publish{}, err
	}
	provider := strings.TrimSpace(in.Provider)
	if provider == "" {
		return Publish{}, fmt.Errorf("provider is required")
	}
	siteURL := strings.TrimSpace(in.SiteURL)
	deployURL := strings.TrimSpace(in.DeployURL)
	if siteURL == "" && deployURL == "" {
		return Publish{}, fmt.Errorf("site or deploy URL is required")
	}
	id, err := newID("pub_")
	if err != nil {
		return Publish{}, err
	}
	row, err := s.q.InsertProjectPublish(ctx, db.InsertProjectPublishParams{
		ID:        id,
		ProjectID: projectID,
		Provider:  provider,
		Target:    strings.TrimSpace(in.Target),
		DeployID:  strings.TrimSpace(in.DeployID),
		SiteUrl:   siteURL,
		DeployUrl: deployURL,
		CreatedAt: timestamptzFromTime(time.Now().UTC()),
	})
	if err != nil {
		return Publish{}, fmt.Errorf("insert publish: %w", err)
	}
	return publishFromDB(row), nil
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

func publishFromDB(row db.ProjectPublish) Publish {
	return Publish{
		ID:        row.ID,
		ProjectID: row.ProjectID,
		Provider:  row.Provider,
		Target:    row.Target,
		DeployID:  row.DeployID,
		SiteURL:   row.SiteUrl,
		DeployURL: row.DeployUrl,
		CreatedAt: timeFromTimestamptz(row.CreatedAt),
	}
}
