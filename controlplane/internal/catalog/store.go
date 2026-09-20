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
	ErrAgentInUse   = errors.New("agent in use")
	ErrAgentLocked  = errors.New("thread agent is locked")
	ErrProjectInUse = errors.New("project in use")
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
			return Provider{}, newProviderNotFound(id)
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
			return Provider{}, newProviderNotFound(id)
		}
		return Provider{}, err
	}
	return providerFromDB(row)
}

func (s *Store) DeleteProvider(ctx context.Context, id string) error {
	if _, err := s.GetProvider(ctx, id); err != nil {
		return err
	}
	now := time.Now().UTC()
	return s.inTx(ctx, func(q *db.Queries) error {
		if err := q.UnlinkAgentsByProvider(ctx, db.UnlinkAgentsByProviderParams{
			ProviderID: &id,
			UpdatedAt:  timestamptzFromTime(now),
		}); err != nil {
			return err
		}
		if err := q.DeleteProvider(ctx, id); err != nil {
			return err
		}
		return nil
	})
}

func (s *Store) ReplaceProviderModels(ctx context.Context, id string, models []ModelInfo, updatedAt time.Time) (Provider, error) {
	modelsJSON, err := marshalModels(models)
	if err != nil {
		return Provider{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Provider{}, err
	}
	defer tx.Rollback(ctx)

	qtx := s.q.WithTx(tx)

	if _, err := qtx.GetProvider(ctx, id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Provider{}, newProviderNotFound(id)
		}
		return Provider{}, err
	}

	if err := rejectOrphanedAgentDefaults(ctx, qtx, id, models); err != nil {
		return Provider{}, err
	}

	now := time.Now().UTC()
	row, err := qtx.UpdateProviderModels(ctx, db.UpdateProviderModelsParams{
		ID:              id,
		Models:          modelsJSON,
		ModelsUpdatedAt: timestamptzFromTime(updatedAt.UTC()),
		UpdatedAt:       timestamptzFromTime(now),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Provider{}, newProviderNotFound(id)
		}
		return Provider{}, err
	}

	if err := tx.Commit(ctx); err != nil {
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
		out = append(out, agentFromJoined(
			row.ID,
			row.Name,
			row.Description,
			row.Version,
			row.ProviderID,
			row.DefaultModel,
			row.ProviderName,
			row.Settings,
			row.CreatedAt,
			row.UpdatedAt,
		))
	}
	return out, nil
}

func (s *Store) GetAgent(ctx context.Context, id string) (Agent, error) {
	row, err := s.q.GetAgent(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Agent{}, newAgentNotFound(id)
		}
		return Agent{}, err
	}
	return agentFromJoined(
		row.ID,
		row.Name,
		row.Description,
		row.Version,
		row.ProviderID,
		row.DefaultModel,
		row.ProviderName,
		row.Settings,
		row.CreatedAt,
		row.UpdatedAt,
	), nil
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
	pid, model := providerID, defaultModel
	row, err := s.q.InsertAgent(ctx, db.InsertAgentParams{
		ID:           id,
		Name:         name,
		Description:  description,
		Version:      1,
		ProviderID:   &pid,
		DefaultModel: &model,
		Settings:     []byte("{}"),
		CreatedAt:    timestamptzFromTime(now),
		UpdatedAt:    timestamptzFromTime(now),
	})
	if err != nil {
		return Agent{}, err
	}
	return agentFromInsertRow(row), nil
}

func (s *Store) UpdateAgent(ctx context.Context, id string, name, description, providerID, defaultModel *string, settings json.RawMessage) (Agent, error) {
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
		pid := *providerID
		current.ProviderID = &pid
	}
	if defaultModel != nil {
		model := *defaultModel
		current.DefaultModel = &model
	}
	if len(settings) > 0 {
		sandboxPatch, err := SandboxFromSettings(settings)
		if err != nil {
			return Agent{}, err
		}
		merged, err := MergeSettingsSandbox(current.Settings, sandboxPatch)
		if err != nil {
			return Agent{}, err
		}
		current.Settings = merged
	}
	switch {
	case current.ProviderID == nil && current.DefaultModel == nil:
		// incomplete: skip provider/model validation
	case current.ProviderID == nil || current.DefaultModel == nil:
		return Agent{}, fmt.Errorf("provider and model must be set together")
	default:
		if err := s.validateProviderAndModel(ctx, *current.ProviderID, *current.DefaultModel); err != nil {
			return Agent{}, err
		}
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
		Settings:     rawOrDefault(current.Settings, "{}"),
		UpdatedAt:    timestamptzFromTime(now),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Agent{}, newAgentNotFound(id)
		}
		return Agent{}, err
	}
	return agentFromUpdateRow(row), nil
}

func (s *Store) DeleteAgent(ctx context.Context, id string) error {
	if _, err := s.GetAgent(ctx, id); err != nil {
		return err
	}
	n, err := s.CountThreadsByAgent(ctx, id)
	if err != nil {
		return err
	}
	if n > 0 {
		return ErrAgentInUse
	}
	if err := s.q.DeleteAgent(ctx, id); err != nil {
		if isFKViolation(err) {
			return ErrAgentInUse
		}
		return err
	}
	return nil
}

func (s *Store) ListThreads(ctx context.Context, projectID string) ([]ThreadListItem, error) {
	var filter *string
	if id := strings.TrimSpace(projectID); id != "" {
		filter = &id
	}
	rows, err := s.q.ListThreads(ctx, filter)
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
			return ThreadDetail{}, newThreadNotFound(id)
		}
		return ThreadDetail{}, fmt.Errorf("get thread: %w", err)
	}
	msgs, err := s.q.ListMessages(ctx, id)
	if err != nil {
		return ThreadDetail{}, fmt.Errorf("list messages: %w", err)
	}
	messages := make([]ThreadMessage, 0, len(msgs))
	for _, m := range msgs {
		tm, err := threadMessageFromDB(m)
		if err != nil {
			return ThreadDetail{}, fmt.Errorf("list messages: %w", err)
		}
		messages = append(messages, tm)
	}
	return ThreadDetail{
		Thread:       threadFromRow(row),
		MessageCount: len(messages),
		Messages:     messages,
	}, nil
}

func (s *Store) CreateThread(ctx context.Context) (Thread, error) {
	return s.CreateThreadForProject(ctx, "")
}

func (s *Store) CreateThreadForProject(ctx context.Context, projectID string) (Thread, error) {
	pid, err := s.resolveProjectID(ctx, projectID)
	if err != nil {
		return Thread{}, err
	}
	id, err := newID("th_")
	if err != nil {
		return Thread{}, err
	}
	now := time.Now().UTC()
	row, err := s.q.InsertThread(ctx, db.InsertThreadParams{
		ID:          id,
		Title:       "Untitled",
		TitleSource: string(TitleSourceAuto),
		ProjectID:   pid,
		CreatedAt:   timestamptzFromTime(now),
		UpdatedAt:   timestamptzFromTime(now),
	})
	if err != nil {
		return Thread{}, fmt.Errorf("create thread: %w", err)
	}
	return threadFromInsertRow(row), nil
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
		UpdatedAt:   timestamptzFromTime(time.Now().UTC()),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Thread{}, newThreadNotFound(id)
		}
		return Thread{}, fmt.Errorf("rename thread: %w", err)
	}
	return threadFromRenameRow(row), nil
}

func (s *Store) SetThreadViewMode(ctx context.Context, id string, viewModeID *string) (Thread, error) {
	row, err := s.q.SetThreadViewMode(ctx, db.SetThreadViewModeParams{
		ID:         id,
		ViewModeID: viewModeID,
		UpdatedAt:  timestamptzFromTime(time.Now().UTC()),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Thread{}, newThreadNotFound(id)
		}
		return Thread{}, fmt.Errorf("set thread view mode: %w", err)
	}
	return threadFromSetViewModeRow(row), nil
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
				return newThreadNotFound(threadID)
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
			UpdatedAt: timestamptzFromTime(time.Now().UTC()),
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
		UpdatedAt:    timestamptzFromTime(time.Now().UTC()),
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return newThreadNotFound(threadID)
		}
		return fmt.Errorf("set thread model: %w", err)
	}
	return nil
}

func (s *Store) CommitTurn(ctx context.Context, threadID, userText string, assistant AssistantTurn) (Thread, error) {
	err := s.inTx(ctx, func(q *db.Queries) error {
		th, err := q.GetThread(ctx, threadID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return newThreadNotFound(threadID)
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
		userParts, err := json.Marshal([]MessagePart{})
		if err != nil {
			return err
		}
		parts := assistant.Parts
		if parts == nil {
			parts = []MessagePart{{Type: "message", Text: assistant.Content}}
		}
		assistantParts, err := json.Marshal(parts)
		if err != nil {
			return err
		}
		if _, err := q.InsertMessage(ctx, db.InsertMessageParams{
			ID:        userID,
			ThreadID:  threadID,
			Role:      "user",
			Content:   userText,
			Position:  pos + 1,
			CreatedAt: timestamptzFromTime(now),
			Parts:     userParts,
		}); err != nil {
			return fmt.Errorf("insert user message: %w", err)
		}
		if _, err := q.InsertMessage(ctx, db.InsertMessageParams{
			ID:           assistantID,
			ThreadID:     threadID,
			Role:         "assistant",
			Content:      assistant.Content,
			Position:     pos + 2,
			CreatedAt:    timestamptzFromTime(now),
			Parts:        assistantParts,
			Model:        nonEmptyPtr(assistant.Model),
			ProviderID:   nonEmptyPtr(assistant.ProviderID),
			ProviderName: nonEmptyPtr(assistant.ProviderName),
			StopReason:   nonEmptyPtr(assistant.StopReason),
		}); err != nil {
			return fmt.Errorf("insert assistant message: %w", err)
		}

		updatedAt := timestamptzFromTime(now)
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

func threadFromFields(
	id, title, titleSource string,
	agentID, currentModel, viewModeID *string,
	projectID string,
	createdAt, updatedAt pgtype.Timestamptz,
) Thread {
	return Thread{
		ID:           id,
		Title:        title,
		TitleSource:  TitleSource(titleSource),
		AgentID:      agentID,
		CurrentModel: currentModel,
		ViewModeID:   viewModeID,
		ProjectID:    projectID,
		CreatedAt:    timeFromTimestamptz(createdAt),
		UpdatedAt:    timeFromTimestamptz(updatedAt),
	}
}

func threadFromRow(row db.GetThreadRow) Thread {
	return threadFromFields(row.ID, row.Title, row.TitleSource, row.AgentID, row.CurrentModel, row.ViewModeID, row.ProjectID, row.CreatedAt, row.UpdatedAt)
}

func threadFromInsertRow(row db.InsertThreadRow) Thread {
	return threadFromFields(row.ID, row.Title, row.TitleSource, row.AgentID, row.CurrentModel, row.ViewModeID, row.ProjectID, row.CreatedAt, row.UpdatedAt)
}

func threadFromRenameRow(row db.RenameThreadRow) Thread {
	return threadFromFields(row.ID, row.Title, row.TitleSource, row.AgentID, row.CurrentModel, row.ViewModeID, row.ProjectID, row.CreatedAt, row.UpdatedAt)
}

func threadFromSetViewModeRow(row db.SetThreadViewModeRow) Thread {
	return threadFromFields(row.ID, row.Title, row.TitleSource, row.AgentID, row.CurrentModel, row.ViewModeID, row.ProjectID, row.CreatedAt, row.UpdatedAt)
}

func threadFromListRow(row db.ListThreadsRow) Thread {
	return threadFromFields(row.ID, row.Title, row.TitleSource, row.AgentID, row.CurrentModel, row.ViewModeID, row.ProjectID, row.CreatedAt, row.UpdatedAt)
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

func rejectOrphanedAgentDefaults(ctx context.Context, q *db.Queries, providerID string, models []ModelInfo) error {
	ids := make(map[string]struct{}, len(models))
	for _, m := range models {
		ids[m.ID] = struct{}{}
	}

	agents, err := q.ListAgentsByProvider(ctx, &providerID)
	if err != nil {
		return err
	}
	for _, a := range agents {
		if a.DefaultModel == nil {
			continue
		}
		if _, ok := ids[*a.DefaultModel]; !ok {
			return fmt.Errorf("cannot refresh models: agent %q still references default model %q", a.Name, *a.DefaultModel)
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

func agentFromInsertRow(row db.InsertAgentRow) Agent {
	return agentFromJoined(
		row.ID,
		row.Name,
		row.Description,
		row.Version,
		row.ProviderID,
		row.DefaultModel,
		nil,
		row.Settings,
		row.CreatedAt,
		row.UpdatedAt,
	)
}

func agentFromUpdateRow(row db.UpdateAgentRow) Agent {
	return agentFromJoined(
		row.ID,
		row.Name,
		row.Description,
		row.Version,
		row.ProviderID,
		row.DefaultModel,
		nil,
		row.Settings,
		row.CreatedAt,
		row.UpdatedAt,
	)
}

func agentFromJoined(
	id, name, description string,
	version int32,
	providerID, defaultModel, providerName *string,
	settings []byte,
	createdAt, updatedAt pgtype.Timestamptz,
) Agent {
	return Agent{
		ID:           id,
		Name:         name,
		Description:  description,
		Version:      int(version),
		ProviderID:   providerID,
		ProviderName: providerName,
		DefaultModel: defaultModel,
		Settings:     rawOrDefault(settings, "{}"),
		CreatedAt:    timeFromTimestamptz(createdAt),
		UpdatedAt:    timeFromTimestamptz(updatedAt),
	}
}

func threadMessageFromDB(m db.Message) (ThreadMessage, error) {
	parts, err := unmarshalMessageParts(m.Parts)
	if err != nil {
		return ThreadMessage{}, err
	}
	return ThreadMessage{
		ID:           m.ID,
		Role:         m.Role,
		Content:      m.Content,
		Position:     int(m.Position),
		CreatedAt:    timeFromTimestamptz(m.CreatedAt),
		Model:        m.Model,
		ProviderID:   m.ProviderID,
		ProviderName: m.ProviderName,
		StopReason:   m.StopReason,
		Parts:        parts,
	}, nil
}

func unmarshalMessageParts(data []byte) ([]MessagePart, error) {
	if len(data) == 0 {
		return []MessagePart{}, nil
	}
	var parts []MessagePart
	if err := json.Unmarshal(data, &parts); err != nil {
		return nil, err
	}
	if parts == nil {
		return []MessagePart{}, nil
	}
	return parts, nil
}

func nonEmptyPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
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
