package catalog

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tryy3/agent-fabric/internal/db"
)

var ErrProviderInUse = errors.New("provider in use")

type Store struct {
	q *db.Queries
}

func Open(pool *pgxpool.Pool) *Store {
	return &Store{q: db.New(pool)}
}

func (s *Store) ListProviders(ctx context.Context) ([]Provider, error) {
	rows, err := s.q.ListProviders(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Provider, 0, len(rows))
	for _, row := range rows {
		p, err := providerFromDB(row)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

func (s *Store) GetProvider(ctx context.Context, id string) (Provider, error) {
	row, err := s.q.GetProvider(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Provider{}, fmt.Errorf("provider %q not found", id)
		}
		return Provider{}, err
	}
	return providerFromDB(row)
}

func (s *Store) CreateProvider(ctx context.Context, name, typ, baseURL, apiKey string) (Provider, error) {
	if strings.TrimSpace(name) == "" {
		return Provider{}, fmt.Errorf("provider name is required")
	}
	if strings.TrimSpace(baseURL) == "" {
		return Provider{}, fmt.Errorf("provider baseURL is required")
	}
	if strings.TrimSpace(apiKey) == "" {
		return Provider{}, fmt.Errorf("provider apiKey is required")
	}
	if !isKnownProviderType(typ) {
		return Provider{}, fmt.Errorf("unknown provider type %q", typ)
	}

	id, err := newID("prov_")
	if err != nil {
		return Provider{}, err
	}

	now := time.Now().UTC()
	modelsJSON, err := marshalModels(nil)
	if err != nil {
		return Provider{}, err
	}

	row, err := s.q.InsertProvider(ctx, db.InsertProviderParams{
		ID:              id,
		Name:            name,
		Type:            typ,
		BaseUrl:         strings.TrimRight(baseURL, "/"),
		ApiKey:          apiKey,
		Models:          modelsJSON,
		ModelsUpdatedAt: pgtype.Timestamptz{},
		CreatedAt:       timestamptzFromTime(now),
		UpdatedAt:       timestamptzFromTime(now),
	})
	if err != nil {
		return Provider{}, err
	}
	return providerFromDB(row)
}

func (s *Store) UpdateProvider(ctx context.Context, id string, name, baseURL, apiKey *string) (Provider, error) {
	current, err := s.GetProvider(ctx, id)
	if err != nil {
		return Provider{}, err
	}

	if name != nil {
		if strings.TrimSpace(*name) == "" {
			return Provider{}, fmt.Errorf("provider name is required")
		}
		current.Name = *name
	}
	if baseURL != nil {
		if strings.TrimSpace(*baseURL) == "" {
			return Provider{}, fmt.Errorf("provider baseURL is required")
		}
		current.BaseURL = strings.TrimRight(*baseURL, "/")
	}
	if apiKey != nil {
		if strings.TrimSpace(*apiKey) == "" {
			return Provider{}, fmt.Errorf("provider apiKey is required")
		}
		current.APIKey = *apiKey
	}

	now := time.Now().UTC()
	row, err := s.q.UpdateProvider(ctx, db.UpdateProviderParams{
		ID:        id,
		Name:      current.Name,
		BaseUrl:   current.BaseURL,
		ApiKey:    current.APIKey,
		UpdatedAt: timestamptzFromTime(now),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Provider{}, fmt.Errorf("provider %q not found", id)
		}
		return Provider{}, err
	}
	return providerFromDB(row)
}

func (s *Store) DeleteProvider(ctx context.Context, id string) error {
	if _, err := s.GetProvider(ctx, id); err != nil {
		return err
	}

	count, err := s.q.CountAgentsByProvider(ctx, id)
	if err != nil {
		return err
	}
	if count > 0 {
		return ErrProviderInUse
	}

	if err := s.q.DeleteProvider(ctx, id); err != nil {
		if isFKViolation(err) {
			return ErrProviderInUse
		}
		return err
	}
	return nil
}

func (s *Store) ReplaceProviderModels(ctx context.Context, id string, models []ModelInfo, updatedAt time.Time) (Provider, error) {
	if _, err := s.GetProvider(ctx, id); err != nil {
		return Provider{}, err
	}

	if err := s.rejectOrphanedAgentDefaults(ctx, id, models); err != nil {
		return Provider{}, err
	}

	modelsJSON, err := marshalModels(models)
	if err != nil {
		return Provider{}, err
	}

	now := time.Now().UTC()
	row, err := s.q.UpdateProviderModels(ctx, db.UpdateProviderModelsParams{
		ID:              id,
		Models:          modelsJSON,
		ModelsUpdatedAt: timestamptzFromTime(updatedAt.UTC()),
		UpdatedAt:       timestamptzFromTime(now),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Provider{}, fmt.Errorf("provider %q not found", id)
		}
		return Provider{}, err
	}
	return providerFromDB(row)
}

func (s *Store) ListAgents(ctx context.Context) ([]Agent, error) {
	rows, err := s.q.ListAgents(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Agent, 0, len(rows))
	for _, row := range rows {
		out = append(out, agentFromDB(row))
	}
	return out, nil
}

func (s *Store) GetAgent(ctx context.Context, id string) (Agent, error) {
	row, err := s.q.GetAgent(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Agent{}, fmt.Errorf("agent %q not found", id)
		}
		return Agent{}, err
	}
	return agentFromDB(row), nil
}

func (s *Store) CreateAgent(ctx context.Context, name, description, providerID, defaultModel string) (Agent, error) {
	if strings.TrimSpace(name) == "" {
		return Agent{}, fmt.Errorf("agent name is required")
	}
	if err := s.validateProviderAndModel(ctx, providerID, defaultModel); err != nil {
		return Agent{}, err
	}

	id, err := newID("agent_")
	if err != nil {
		return Agent{}, err
	}

	now := time.Now().UTC()
	row, err := s.q.InsertAgent(ctx, db.InsertAgentParams{
		ID:           id,
		Name:         name,
		Description:  description,
		Version:      1,
		ProviderID:   providerID,
		DefaultModel: defaultModel,
		CreatedAt:    timestamptzFromTime(now),
		UpdatedAt:    timestamptzFromTime(now),
	})
	if err != nil {
		return Agent{}, err
	}
	return agentFromDB(row), nil
}

func (s *Store) UpdateAgent(ctx context.Context, id string, name, description, providerID, defaultModel *string) (Agent, error) {
	current, err := s.GetAgent(ctx, id)
	if err != nil {
		return Agent{}, err
	}

	if name != nil {
		if strings.TrimSpace(*name) == "" {
			return Agent{}, fmt.Errorf("agent name is required")
		}
		current.Name = *name
	}
	if description != nil {
		current.Description = *description
	}
	if providerID != nil {
		current.ProviderID = *providerID
	}
	if defaultModel != nil {
		current.DefaultModel = *defaultModel
	}
	if err := s.validateProviderAndModel(ctx, current.ProviderID, current.DefaultModel); err != nil {
		return Agent{}, err
	}

	current.Version++
	now := time.Now().UTC()
	row, err := s.q.UpdateAgent(ctx, db.UpdateAgentParams{
		ID:           id,
		Name:         current.Name,
		Description:  current.Description,
		Version:      int32(current.Version),
		ProviderID:   current.ProviderID,
		DefaultModel: current.DefaultModel,
		UpdatedAt:    timestamptzFromTime(now),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Agent{}, fmt.Errorf("agent %q not found", id)
		}
		return Agent{}, err
	}
	return agentFromDB(row), nil
}

func (s *Store) DeleteAgent(ctx context.Context, id string) error {
	if _, err := s.GetAgent(ctx, id); err != nil {
		return err
	}
	return s.q.DeleteAgent(ctx, id)
}

func (s *Store) validateProviderAndModel(ctx context.Context, providerID, defaultModel string) error {
	p, err := s.GetProvider(ctx, providerID)
	if err != nil {
		return err
	}
	for _, m := range p.Models {
		if m.ID == defaultModel {
			return nil
		}
	}
	return fmt.Errorf("model %q not found for provider %q", defaultModel, providerID)
}

func (s *Store) rejectOrphanedAgentDefaults(ctx context.Context, providerID string, models []ModelInfo) error {
	ids := make(map[string]struct{}, len(models))
	for _, m := range models {
		ids[m.ID] = struct{}{}
	}

	agents, err := s.q.ListAgents(ctx)
	if err != nil {
		return err
	}
	for _, a := range agents {
		if a.ProviderID != providerID {
			continue
		}
		if _, ok := ids[a.DefaultModel]; !ok {
			return fmt.Errorf("cannot refresh models: agent %q still references default model %q", a.Name, a.DefaultModel)
		}
	}
	return nil
}

func providerFromDB(row db.Provider) (Provider, error) {
	models, err := unmarshalModels(row.Models)
	if err != nil {
		return Provider{}, err
	}
	return Provider{
		ID:              row.ID,
		Name:            row.Name,
		Type:            row.Type,
		BaseURL:         row.BaseUrl,
		APIKey:          row.ApiKey,
		Models:          models,
		ModelsUpdatedAt: timePtrFromTimestamptz(row.ModelsUpdatedAt),
		CreatedAt:       timeFromTimestamptz(row.CreatedAt),
		UpdatedAt:       timeFromTimestamptz(row.UpdatedAt),
	}, nil
}

func agentFromDB(row db.Agent) Agent {
	return Agent{
		ID:           row.ID,
		Name:         row.Name,
		Description:  row.Description,
		Version:      int(row.Version),
		ProviderID:   row.ProviderID,
		DefaultModel: row.DefaultModel,
		CreatedAt:    timeFromTimestamptz(row.CreatedAt),
		UpdatedAt:    timeFromTimestamptz(row.UpdatedAt),
	}
}

func marshalModels(models []ModelInfo) ([]byte, error) {
	if models == nil {
		models = []ModelInfo{}
	}
	return json.Marshal(models)
}

func unmarshalModels(data []byte) ([]ModelInfo, error) {
	if len(data) == 0 {
		return []ModelInfo{}, nil
	}
	var models []ModelInfo
	if err := json.Unmarshal(data, &models); err != nil {
		return nil, err
	}
	if models == nil {
		return []ModelInfo{}, nil
	}
	return models, nil
}

func timestamptzFromTime(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t.UTC(), Valid: true}
}

func timeFromTimestamptz(t pgtype.Timestamptz) time.Time {
	if !t.Valid {
		return time.Time{}
	}
	return t.Time.UTC()
}

func timePtrFromTimestamptz(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	tt := t.Time.UTC()
	return &tt
}

func isFKViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}

func isKnownProviderType(typ string) bool {
	return typ == TypeOpenAICompatible
}

func newID(prefix string) (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(b), nil
}
