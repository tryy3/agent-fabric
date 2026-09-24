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

func (s *Store) InsertCheckpoint(ctx context.Context, projectID, sha, label string, threadID, messageID *string) (Checkpoint, error) {
	if _, err := s.GetProject(ctx, projectID); err != nil {
		return Checkpoint{}, err
	}
	sha = strings.TrimSpace(sha)
	if sha == "" {
		return Checkpoint{}, fmt.Errorf("sha is required")
	}
	label = strings.TrimSpace(label)
	if label == "" {
		return Checkpoint{}, fmt.Errorf("label is required")
	}
	id, err := newID("chk_")
	if err != nil {
		return Checkpoint{}, err
	}
	now := time.Now().UTC()
	row, err := s.q.InsertProjectCheckpoint(ctx, db.InsertProjectCheckpointParams{
		ID:        id,
		ProjectID: projectID,
		Sha:       sha,
		Label:     label,
		ThreadID:  emptyToNil(threadID),
		MessageID: emptyToNil(messageID),
		CreatedAt: timestamptzFromTime(now),
	})
	if err != nil {
		return Checkpoint{}, fmt.Errorf("insert checkpoint: %w", err)
	}
	return checkpointFromDB(row), nil
}

func (s *Store) ListCheckpoints(ctx context.Context, projectID string) ([]Checkpoint, error) {
	if _, err := s.GetProject(ctx, projectID); err != nil {
		return nil, err
	}
	rows, err := s.q.ListProjectCheckpoints(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list checkpoints: %w", err)
	}
	out := make([]Checkpoint, 0, len(rows))
	for _, row := range rows {
		out = append(out, checkpointFromDB(row))
	}
	return out, nil
}

func (s *Store) GetCheckpoint(ctx context.Context, id string) (Checkpoint, error) {
	row, err := s.q.GetProjectCheckpoint(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Checkpoint{}, fmt.Errorf("checkpoint %q not found", id)
		}
		return Checkpoint{}, fmt.Errorf("get checkpoint: %w", err)
	}
	return checkpointFromDB(row), nil
}

func checkpointFromDB(row db.ProjectCheckpoint) Checkpoint {
	return Checkpoint{
		ID:        row.ID,
		ProjectID: row.ProjectID,
		SHA:       row.Sha,
		Label:     row.Label,
		ThreadID:  row.ThreadID,
		MessageID: row.MessageID,
		CreatedAt: timeFromTimestamptz(row.CreatedAt),
	}
}

func emptyToNil(v *string) *string {
	if v == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*v)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
