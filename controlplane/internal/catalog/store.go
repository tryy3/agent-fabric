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

var (
	ErrProviderInUse = errors.New("provider in use")
	ErrAgentLocked   = errors.New("thread agent is locked")
)

type Store struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

func Open(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool, q: db.New(pool)}
}

func (s *Store) inTx(ctx context.Context, fn func(*db.Queries) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(s.q.WithTx(tx)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
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
	return getProvider(ctx, s.q, id)
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
	var out Provider
	err := s.inTx(ctx, func(q *db.Queries) error {
		p, err := getProviderForUpdate(ctx, q, id)
		if err != nil {
			return err
		}

		if name != nil {
			if strings.TrimSpace(*name) == "" {
				return fmt.Errorf("provider name is required")
			}
			p.Name = *name
		}
		if baseURL != nil {
			if strings.TrimSpace(*baseURL) == "" {
				return fmt.Errorf("provider baseURL is required")
			}
			p.BaseURL = strings.TrimRight(*baseURL, "/")
		}
		if apiKey != nil {
			if strings.TrimSpace(*apiKey) == "" {
				return fmt.Errorf("provider apiKey is required")
			}
			p.APIKey = *apiKey
		}

		row, err := q.UpdateProvider(ctx, db.UpdateProviderParams{
			ID:        id,
			Name:      p.Name,
			BaseUrl:   p.BaseURL,
			ApiKey:    p.APIKey,
			UpdatedAt: stamp(time.Now().UTC()),
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("provider %q not found", id)
			}
			return fmt.Errorf("update provider: %w", err)
		}
		out, err = providerFromRow(row)
		return err
	})
	return out, err
}

func (s *Store) DeleteProvider(ctx context.Context, id string) error {
	return s.inTx(ctx, func(q *db.Queries) error {
		if _, err := getProviderForUpdate(ctx, q, id); err != nil {
			return err
		}

		n, err := q.CountAgentsByProvider(ctx, id)
		if err != nil {
			return fmt.Errorf("count agents by provider: %w", err)
		}
		if n > 0 {
			return ErrProviderInUse
		}

		if err := q.DeleteProvider(ctx, id); err != nil {
			if isFKViolation(err) {
				return ErrProviderInUse
			}
			return fmt.Errorf("delete provider: %w", err)
		}
		return nil
	})
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
	return getAgent(ctx, s.q, id)
}

func (s *Store) CreateAgent(ctx context.Context, name, description, providerID, defaultModel string) (Agent, error) {
	if strings.TrimSpace(name) == "" {
		return Agent{}, fmt.Errorf("agent name is required")
	}

	id, err := newID("agent_")
	if err != nil {
		return Agent{}, err
	}

	var out Agent
	err = s.inTx(ctx, func(q *db.Queries) error {
		if err := validateProviderAndModel(ctx, q, providerID, defaultModel); err != nil {
			return err
		}

		now := time.Now().UTC()
		row, err := q.InsertAgent(ctx, db.InsertAgentParams{
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
			return fmt.Errorf("create agent: %w", err)
		}
		out = agentFromRow(row)
		return nil
	})
	return out, err
}

func (s *Store) UpdateAgent(ctx context.Context, id string, name, description, providerID, defaultModel *string) (Agent, error) {
	var out Agent
	err := s.inTx(ctx, func(q *db.Queries) error {
		a, err := getAgentForUpdate(ctx, q, id)
		if err != nil {
			return err
		}

		if name != nil {
			if strings.TrimSpace(*name) == "" {
				return fmt.Errorf("agent name is required")
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
		if err := validateProviderAndModel(ctx, q, a.ProviderID, a.DefaultModel); err != nil {
			return err
		}

		row, err := q.UpdateAgent(ctx, db.UpdateAgentParams{
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
				return fmt.Errorf("agent %q not found", id)
			}
			return fmt.Errorf("update agent: %w", err)
		}
		out = agentFromRow(row)
		return nil
	})
	return out, err
}

func (s *Store) DeleteAgent(ctx context.Context, id string) error {
	return s.inTx(ctx, func(q *db.Queries) error {
		if _, err := getAgentForUpdate(ctx, q, id); err != nil {
			return err
		}
		if err := q.DeleteAgent(ctx, id); err != nil {
			return fmt.Errorf("delete agent: %w", err)
		}
		return nil
	})
}

func (s *Store) ReplaceProviderModels(ctx context.Context, id string, models []ModelInfo, updatedAt time.Time) (Provider, error) {
	var out Provider
	err := s.inTx(ctx, func(q *db.Queries) error {
		if _, err := getProviderForUpdate(ctx, q, id); err != nil {
			return err
		}
		if err := rejectOrphanedAgentDefaults(ctx, q, id, models); err != nil {
			return err
		}

		raw, err := marshalModels(models)
		if err != nil {
			return err
		}
		row, err := q.UpdateProviderModels(ctx, db.UpdateProviderModelsParams{
			ID:              id,
			Models:          raw,
			ModelsUpdatedAt: stamp(updatedAt.UTC()),
			UpdatedAt:       stamp(time.Now().UTC()),
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("provider %q not found", id)
			}
			return fmt.Errorf("update provider models: %w", err)
		}
		out, err = providerFromRow(row)
		return err
	})
	return out, err
}

func (s *Store) ListThreads(ctx context.Context) ([]ThreadListItem, error) {
	rows, err := s.q.ListThreads(ctx)
	if err != nil {
		return nil, fmt.Errorf("list threads: %w", err)
	}
	out := make([]ThreadListItem, 0, len(rows))
	for _, row := range rows {
		out = append(out, ThreadListItem{
			Thread:       threadFromListRow(row),
			MessageCount: int(row.MessageCount),
		})
	}
	return out, nil
}

func (s *Store) GetThread(ctx context.Context, id string) (ThreadDetail, error) {
	row, err := s.q.GetThread(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ThreadDetail{}, fmt.Errorf("thread %q not found", id)
		}
		return ThreadDetail{}, fmt.Errorf("get thread: %w", err)
	}
	msgs, err := s.q.ListMessages(ctx, id)
	if err != nil {
		return ThreadDetail{}, fmt.Errorf("list messages: %w", err)
	}
	messages := make([]ThreadMessage, 0, len(msgs))
	for _, m := range msgs {
		messages = append(messages, ThreadMessage{
			ID:        m.ID,
			Role:      m.Role,
			Content:   m.Content,
			Position:  int(m.Position),
			CreatedAt: m.CreatedAt.Time.UTC(),
		})
	}
	return ThreadDetail{Thread: threadFromRow(row), Messages: messages}, nil
}

func (s *Store) CreateThread(ctx context.Context) (Thread, error) {
	id, err := newID("th_")
	if err != nil {
		return Thread{}, err
	}
	now := time.Now().UTC()
	row, err := s.q.InsertThread(ctx, db.InsertThreadParams{
		ID:          id,
		Title:       "Untitled",
		TitleSource: string(TitleSourceAuto),
		CreatedAt:   stamp(now),
		UpdatedAt:   stamp(now),
	})
	if err != nil {
		return Thread{}, fmt.Errorf("create thread: %w", err)
	}
	return threadFromRow(row), nil
}

func (s *Store) RenameThread(ctx context.Context, id, title string) (Thread, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return Thread{}, fmt.Errorf("thread title is required")
	}
	row, err := s.q.RenameThread(ctx, db.RenameThreadParams{
		ID:          id,
		Title:       title,
		TitleSource: string(TitleSourceUser),
		UpdatedAt:   stamp(time.Now().UTC()),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Thread{}, fmt.Errorf("thread %q not found", id)
		}
		return Thread{}, fmt.Errorf("rename thread: %w", err)
	}
	return threadFromRow(row), nil
}

func (s *Store) CountThreadsByAgent(ctx context.Context, agentID string) (int64, error) {
	n, err := s.q.CountThreadsByAgent(ctx, &agentID)
	if err != nil {
		return 0, fmt.Errorf("count threads by agent: %w", err)
	}
	return n, nil
}

func (s *Store) PinThreadAgent(ctx context.Context, threadID, agentID string) error {
	return s.inTx(ctx, func(q *db.Queries) error {
		row, err := q.GetThreadForUpdate(ctx, threadID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("thread %q not found", threadID)
			}
			return fmt.Errorf("get thread: %w", err)
		}
		if row.AgentID != nil {
			if *row.AgentID == agentID {
				return nil
			}
			return ErrAgentLocked
		}
		if _, err := q.PinThreadAgent(ctx, db.PinThreadAgentParams{
			ID:        threadID,
			AgentID:   &agentID,
			UpdatedAt: stamp(time.Now().UTC()),
		}); err != nil {
			return fmt.Errorf("pin thread agent: %w", err)
		}
		return nil
	})
}

func (s *Store) SetThreadModel(ctx context.Context, threadID, model string) error {
	if _, err := s.q.SetThreadModel(ctx, db.SetThreadModelParams{
		ID:           threadID,
		CurrentModel: &model,
		UpdatedAt:    stamp(time.Now().UTC()),
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("thread %q not found", threadID)
		}
		return fmt.Errorf("set thread model: %w", err)
	}
	return nil
}

func (s *Store) CommitTurn(ctx context.Context, threadID, userText, assistantText string) (Thread, error) {
	err := s.inTx(ctx, func(q *db.Queries) error {
		th, err := q.GetThread(ctx, threadID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("thread %q not found", threadID)
			}
			return fmt.Errorf("get thread: %w", err)
		}

		pos, err := q.NextMessagePosition(ctx, threadID)
		if err != nil {
			return fmt.Errorf("next message position: %w", err)
		}
		now := time.Now().UTC()
		userID, err := newID("msg_")
		if err != nil {
			return err
		}
		assistantID, err := newID("msg_")
		if err != nil {
			return err
		}
		if _, err := q.InsertMessage(ctx, db.InsertMessageParams{
			ID:        userID,
			ThreadID:  threadID,
			Role:      "user",
			Content:   userText,
			Position:  pos + 1,
			CreatedAt: stamp(now),
		}); err != nil {
			return fmt.Errorf("insert user message: %w", err)
		}
		if _, err := q.InsertMessage(ctx, db.InsertMessageParams{
			ID:        assistantID,
			ThreadID:  threadID,
			Role:      "assistant",
			Content:   assistantText,
			Position:  pos + 2,
			CreatedAt: stamp(now),
		}); err != nil {
			return fmt.Errorf("insert assistant message: %w", err)
		}

		updatedAt := stamp(now)
		if TitleSource(th.TitleSource) == TitleSourceAuto && pos == -1 {
			if err := q.SetThreadTitleIfAuto(ctx, db.SetThreadTitleIfAutoParams{
				ID:        threadID,
				Title:     AutoTitle(userText),
				UpdatedAt: updatedAt,
			}); err != nil {
				return fmt.Errorf("set thread title: %w", err)
			}
			return nil
		}
		if err := q.TouchThread(ctx, db.TouchThreadParams{
			ID:        threadID,
			UpdatedAt: updatedAt,
		}); err != nil {
			return fmt.Errorf("touch thread: %w", err)
		}
		return nil
	})
	if err != nil {
		return Thread{}, err
	}
	detail, err := s.GetThread(ctx, threadID)
	if err != nil {
		return Thread{}, err
	}
	return detail.Thread, nil
}

func threadFromRow(row db.Thread) Thread {
	return Thread{
		ID:           row.ID,
		Title:        row.Title,
		TitleSource:  TitleSource(row.TitleSource),
		AgentID:      row.AgentID,
		CurrentModel: row.CurrentModel,
		CreatedAt:    row.CreatedAt.Time.UTC(),
		UpdatedAt:    row.UpdatedAt.Time.UTC(),
	}
}

func threadFromListRow(row db.ListThreadsRow) Thread {
	return Thread{
		ID:           row.ID,
		Title:        row.Title,
		TitleSource:  TitleSource(row.TitleSource),
		AgentID:      row.AgentID,
		CurrentModel: row.CurrentModel,
		CreatedAt:    row.CreatedAt.Time.UTC(),
		UpdatedAt:    row.UpdatedAt.Time.UTC(),
	}
}

func validateProviderAndModel(ctx context.Context, q *db.Queries, providerID, defaultModel string) error {
	p, err := getProviderForUpdate(ctx, q, providerID)
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

func rejectOrphanedAgentDefaults(ctx context.Context, q *db.Queries, providerID string, models []ModelInfo) error {
	ids := make(map[string]struct{}, len(models))
	for _, m := range models {
		ids[m.ID] = struct{}{}
	}
	agents, err := q.ListAgentsByProvider(ctx, providerID)
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

func getProvider(ctx context.Context, q *db.Queries, id string) (Provider, error) {
	row, err := q.GetProvider(ctx, id)
	return providerFromGet(id, row, err)
}

func getProviderForUpdate(ctx context.Context, q *db.Queries, id string) (Provider, error) {
	row, err := q.GetProviderForUpdate(ctx, id)
	return providerFromGet(id, row, err)
}

func providerFromGet(id string, row db.Provider, err error) (Provider, error) {
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Provider{}, fmt.Errorf("provider %q not found", id)
		}
		return Provider{}, fmt.Errorf("get provider: %w", err)
	}
	return providerFromRow(row)
}

func getAgent(ctx context.Context, q *db.Queries, id string) (Agent, error) {
	row, err := q.GetAgent(ctx, id)
	return agentFromGet(id, row, err)
}

func getAgentForUpdate(ctx context.Context, q *db.Queries, id string) (Agent, error) {
	row, err := q.GetAgentForUpdate(ctx, id)
	return agentFromGet(id, row, err)
}

func agentFromGet(id string, row db.Agent, err error) (Agent, error) {
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Agent{}, fmt.Errorf("agent %q not found", id)
		}
		return Agent{}, fmt.Errorf("get agent: %w", err)
	}
	return agentFromRow(row), nil
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
