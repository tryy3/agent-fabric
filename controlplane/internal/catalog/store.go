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
		return nil, fmt.Errorf("list providers: %w", err)
	}
	out := make([]Provider, 0, len(rows))
	for _, row := range rows {
		p, err := providerFromRow(row)
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
		return Provider{}, fmt.Errorf("get provider: %w", err)
	}
	return providerFromRow(row)
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
	models, err := marshalModels([]ModelInfo{})
	if err != nil {
		return Provider{}, err
	}
	row, err := s.q.InsertProvider(ctx, db.InsertProviderParams{
		ID:        id,
		Name:      name,
		Type:      typ,
		BaseUrl:   strings.TrimRight(baseURL, "/"),
		ApiKey:    apiKey,
		Models:    models,
		CreatedAt: stamp(now),
		UpdatedAt: stamp(now),
	})
	if err != nil {
		return Provider{}, fmt.Errorf("create provider: %w", err)
	}
	return providerFromRow(row)
}

func (s *Store) UpdateProvider(ctx context.Context, id string, name, baseURL, apiKey *string) (Provider, error) {
	p, err := s.GetProvider(ctx, id)
	if err != nil {
		return Provider{}, err
	}

	if name != nil {
		if strings.TrimSpace(*name) == "" {
			return Provider{}, fmt.Errorf("provider name is required")
		}
		p.Name = *name
	}
	if baseURL != nil {
		if strings.TrimSpace(*baseURL) == "" {
			return Provider{}, fmt.Errorf("provider baseURL is required")
		}
		p.BaseURL = strings.TrimRight(*baseURL, "/")
	}
	if apiKey != nil {
		if strings.TrimSpace(*apiKey) == "" {
			return Provider{}, fmt.Errorf("provider apiKey is required")
		}
		p.APIKey = *apiKey
	}

	row, err := s.q.UpdateProvider(ctx, db.UpdateProviderParams{
		ID:        id,
		Name:      p.Name,
		BaseUrl:   p.BaseURL,
		ApiKey:    p.APIKey,
		UpdatedAt: stamp(time.Now().UTC()),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Provider{}, fmt.Errorf("provider %q not found", id)
		}
		return Provider{}, fmt.Errorf("update provider: %w", err)
	}
	return providerFromRow(row)
}

func (s *Store) DeleteProvider(ctx context.Context, id string) error {
	if _, err := s.GetProvider(ctx, id); err != nil {
		return err
	}

	n, err := s.q.CountAgentsByProvider(ctx, id)
	if err != nil {
		return fmt.Errorf("count agents by provider: %w", err)
	}
	if n > 0 {
		return ErrProviderInUse
	}

	if err := s.q.DeleteProvider(ctx, id); err != nil {
		if isFKViolation(err) {
			return ErrProviderInUse
		}
		return fmt.Errorf("delete provider: %w", err)
	}
	return nil
}

func (s *Store) ListAgents(ctx context.Context) ([]Agent, error) {
	rows, err := s.q.ListAgents(ctx)
	if err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}
	out := make([]Agent, 0, len(rows))
	for _, row := range rows {
		out = append(out, agentFromRow(row))
	}
	return out, nil
}

func (s *Store) GetAgent(ctx context.Context, id string) (Agent, error) {
	row, err := s.q.GetAgent(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Agent{}, fmt.Errorf("agent %q not found", id)
		}
		return Agent{}, fmt.Errorf("get agent: %w", err)
	}
	return agentFromRow(row), nil
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
		CreatedAt:    stamp(now),
		UpdatedAt:    stamp(now),
	})
	if err != nil {
		return Agent{}, fmt.Errorf("create agent: %w", err)
	}
	return agentFromRow(row), nil
}

func (s *Store) UpdateAgent(ctx context.Context, id string, name, description, providerID, defaultModel *string) (Agent, error) {
	a, err := s.GetAgent(ctx, id)
	if err != nil {
		return Agent{}, err
	}

	if name != nil {
		if strings.TrimSpace(*name) == "" {
			return Agent{}, fmt.Errorf("agent name is required")
		}
		a.Name = *name
	}
	if description != nil {
		a.Description = *description
	}
	if providerID != nil {
		a.ProviderID = *providerID
	}
	if defaultModel != nil {
		a.DefaultModel = *defaultModel
	}
	if err := s.validateProviderAndModel(ctx, a.ProviderID, a.DefaultModel); err != nil {
		return Agent{}, err
	}

	row, err := s.q.UpdateAgent(ctx, db.UpdateAgentParams{
		ID:           id,
		Name:         a.Name,
		Description:  a.Description,
		Version:      int32(a.Version + 1),
		ProviderID:   a.ProviderID,
		DefaultModel: a.DefaultModel,
		UpdatedAt:    stamp(time.Now().UTC()),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Agent{}, fmt.Errorf("agent %q not found", id)
		}
		return Agent{}, fmt.Errorf("update agent: %w", err)
	}
	return agentFromRow(row), nil
}

func (s *Store) DeleteAgent(ctx context.Context, id string) error {
	if _, err := s.GetAgent(ctx, id); err != nil {
		return err
	}
	if err := s.q.DeleteAgent(ctx, id); err != nil {
		return fmt.Errorf("delete agent: %w", err)
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

	raw, err := marshalModels(models)
	if err != nil {
		return Provider{}, err
	}
	row, err := s.q.UpdateProviderModels(ctx, db.UpdateProviderModelsParams{
		ID:              id,
		Models:          raw,
		ModelsUpdatedAt: stamp(updatedAt.UTC()),
		UpdatedAt:       stamp(time.Now().UTC()),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Provider{}, fmt.Errorf("provider %q not found", id)
		}
		return Provider{}, fmt.Errorf("update provider models: %w", err)
	}
	return providerFromRow(row)
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
	agents, err := s.q.ListAgentsByProvider(ctx, providerID)
	if err != nil {
		return fmt.Errorf("list agents by provider: %w", err)
	}
	for _, a := range agents {
		if _, ok := ids[a.DefaultModel]; !ok {
			return fmt.Errorf("cannot refresh models: agent %q still references default model %q", a.Name, a.DefaultModel)
		}
	}
	return nil
}

func providerFromRow(row db.Provider) (Provider, error) {
	models, err := unmarshalModels(row.Models)
	if err != nil {
		return Provider{}, err
	}
	p := Provider{
		ID:        row.ID,
		Name:      row.Name,
		Type:      row.Type,
		BaseURL:   row.BaseUrl,
		APIKey:    row.ApiKey,
		Models:    models,
		CreatedAt: row.CreatedAt.Time.UTC(),
		UpdatedAt: row.UpdatedAt.Time.UTC(),
	}
	if row.ModelsUpdatedAt.Valid {
		t := row.ModelsUpdatedAt.Time.UTC()
		p.ModelsUpdatedAt = &t
	}
	return p, nil
}

func agentFromRow(row db.Agent) Agent {
	return Agent{
		ID:           row.ID,
		Name:         row.Name,
		Description:  row.Description,
		Version:      int(row.Version),
		ProviderID:   row.ProviderID,
		DefaultModel: row.DefaultModel,
		CreatedAt:    row.CreatedAt.Time.UTC(),
		UpdatedAt:    row.UpdatedAt.Time.UTC(),
	}
}

func marshalModels(models []ModelInfo) ([]byte, error) {
	if models == nil {
		models = []ModelInfo{}
	}
	data, err := json.Marshal(models)
	if err != nil {
		return nil, fmt.Errorf("marshal models: %w", err)
	}
	return data, nil
}

func unmarshalModels(data []byte) ([]ModelInfo, error) {
	if len(data) == 0 {
		return []ModelInfo{}, nil
	}
	var models []ModelInfo
	if err := json.Unmarshal(data, &models); err != nil {
		return nil, fmt.Errorf("unmarshal models: %w", err)
	}
	if models == nil {
		return []ModelInfo{}, nil
	}
	return models, nil
}

func stamp(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
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
