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
	ErrAssistantInUse           = errors.New("assistant in use")
	ErrAssistantLocked          = errors.New("thread assistant is locked")
	ErrProjectInUse         = errors.New("project in use")
	ErrDefaultProject       = errors.New("default project cannot be deleted")
	ErrDefaultProjectRename = errors.New("default project cannot be renamed")
)

type Store struct {
	pool           *pgxpool.Pool
	q              *db.Queries
	IdentityPrefix string
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

func (s *Store) ListInferenceConnections(ctx context.Context) ([]InferenceConnection, error) {
	rows, err := s.q.ListInferenceConnections(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]InferenceConnection, 0, len(rows))
	for _, row := range rows {
		p, err := providerFromDB(row)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

func (s *Store) GetInferenceConnection(ctx context.Context, id string) (InferenceConnection, error) {
	row, err := s.q.GetInferenceConnection(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return InferenceConnection{}, newInferenceConnectionNotFound(id)
		}
		return InferenceConnection{}, err
	}
	return providerFromDB(row)
}

func (s *Store) CreateInferenceConnection(ctx context.Context, name, typ, baseURL, apiKey string) (InferenceConnection, error) {
	if !isKnownProviderType(typ) {
		return InferenceConnection{}, fmt.Errorf("unknown provider type %q", typ)
	}
	if strings.TrimSpace(apiKey) == "" {
		return InferenceConnection{}, fmt.Errorf("provider apiKey is required")
	}

	name = strings.TrimSpace(name)
	if name == "" {
		name = DefaultInferenceConnectionName(typ)
	}
	if name == "" {
		return InferenceConnection{}, fmt.Errorf("provider name is required")
	}

	if fixed := FixedBaseURL(typ); fixed != "" {
		baseURL = fixed
	} else if strings.TrimSpace(baseURL) == "" {
		return InferenceConnection{}, fmt.Errorf("provider baseURL is required")
	}

	id, err := newID("prov_")
	if err != nil {
		return InferenceConnection{}, err
	}

	now := time.Now().UTC()
	modelsJSON, err := marshalModels(nil)
	if err != nil {
		return InferenceConnection{}, err
	}

	row, err := s.q.InsertInferenceConnection(ctx, db.InsertInferenceConnectionParams{
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
		return InferenceConnection{}, err
	}
	return providerFromDB(row)
}

func (s *Store) UpdateInferenceConnection(ctx context.Context, id string, name, baseURL, apiKey *string) (InferenceConnection, error) {
	current, err := s.GetInferenceConnection(ctx, id)
	if err != nil {
		return InferenceConnection{}, err
	}

	if name != nil {
		if strings.TrimSpace(*name) == "" {
			return InferenceConnection{}, fmt.Errorf("provider name is required")
		}
		current.Name = *name
	}
	if baseURL != nil && !IsOpenCodeType(current.Type) {
		if strings.TrimSpace(*baseURL) == "" {
			return InferenceConnection{}, fmt.Errorf("provider baseURL is required")
		}
		current.BaseURL = strings.TrimRight(*baseURL, "/")
	}
	if apiKey != nil {
		if strings.TrimSpace(*apiKey) == "" {
			return InferenceConnection{}, fmt.Errorf("provider apiKey is required")
		}
		current.APIKey = *apiKey
	}
	if fixed := FixedBaseURL(current.Type); fixed != "" {
		current.BaseURL = fixed
	}

	now := time.Now().UTC()
	row, err := s.q.UpdateInferenceConnection(ctx, db.UpdateInferenceConnectionParams{
		ID:        id,
		Name:      current.Name,
		BaseUrl:   current.BaseURL,
		ApiKey:    current.APIKey,
		UpdatedAt: timestamptzFromTime(now),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return InferenceConnection{}, newInferenceConnectionNotFound(id)
		}
		return InferenceConnection{}, err
	}
	return providerFromDB(row)
}

func (s *Store) DeleteInferenceConnection(ctx context.Context, id string) error {
	if _, err := s.GetInferenceConnection(ctx, id); err != nil {
		return err
	}
	now := time.Now().UTC()
	return s.inTx(ctx, func(q *db.Queries) error {
		if err := q.UnlinkAssistantsByInferenceConnection(ctx, db.UnlinkAssistantsByInferenceConnectionParams{
			InferenceConnectionID: &id,
			UpdatedAt:  timestamptzFromTime(now),
		}); err != nil {
			return err
		}
		if err := q.DeleteInferenceConnection(ctx, id); err != nil {
			return err
		}
		return nil
	})
}

func (s *Store) ReplaceInferenceConnectionModels(ctx context.Context, id string, models []ModelInfo, updatedAt time.Time) (InferenceConnection, error) {
	modelsJSON, err := marshalModels(models)
	if err != nil {
		return InferenceConnection{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return InferenceConnection{}, err
	}
	defer tx.Rollback(ctx)

	qtx := s.q.WithTx(tx)

	if _, err := qtx.GetInferenceConnection(ctx, id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return InferenceConnection{}, newInferenceConnectionNotFound(id)
		}
		return InferenceConnection{}, err
	}

	if err := rejectOrphanedAgentDefaults(ctx, qtx, id, models); err != nil {
		return InferenceConnection{}, err
	}

	now := time.Now().UTC()
	row, err := qtx.UpdateInferenceConnectionModels(ctx, db.UpdateInferenceConnectionModelsParams{
		ID:              id,
		Models:          modelsJSON,
		ModelsUpdatedAt: timestamptzFromTime(updatedAt.UTC()),
		UpdatedAt:       timestamptzFromTime(now),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return InferenceConnection{}, newInferenceConnectionNotFound(id)
		}
		return InferenceConnection{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return InferenceConnection{}, err
	}
	return providerFromDB(row)
}

func (s *Store) ListAssistants(ctx context.Context) ([]Assistant, error) {
	rows, err := s.q.ListAssistants(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Assistant, 0, len(rows))
	for _, row := range rows {
		out = append(out, agentFromJoined(
			row.ID,
			row.Name,
			row.Description,
			row.Version,
			row.InferenceConnectionID,
			row.DefaultModel,
			row.InferenceConnectionName,
			row.Settings,
			row.CreatedAt,
			row.UpdatedAt,
		))
	}
	return out, nil
}

func (s *Store) GetAssistant(ctx context.Context, id string) (Assistant, error) {
	row, err := s.q.GetAssistant(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Assistant{}, newAssistantNotFound(id)
		}
		return Assistant{}, err
	}
	return agentFromJoined(
		row.ID,
		row.Name,
		row.Description,
		row.Version,
		row.InferenceConnectionID,
		row.DefaultModel,
		row.InferenceConnectionName,
		row.Settings,
		row.CreatedAt,
		row.UpdatedAt,
	), nil
}

func (s *Store) CreateAssistant(ctx context.Context, name, description, providerID, defaultModel string) (Assistant, error) {
	if strings.TrimSpace(name) == "" {
		return Assistant{}, fmt.Errorf("agent name is required")
	}
	if err := s.validateInferenceConnectionAndModel(ctx, providerID, defaultModel); err != nil {
		return Assistant{}, err
	}

	id, err := newID("agent_")
	if err != nil {
		return Assistant{}, err
	}

	now := time.Now().UTC()
	pid, model := providerID, defaultModel
	row, err := s.q.InsertAssistant(ctx, db.InsertAssistantParams{
		ID:           id,
		Name:         name,
		Description:  description,
		Version:      1,
		InferenceConnectionID:   &pid,
		DefaultModel: &model,
		Settings:     []byte("{}"),
		CreatedAt:    timestamptzFromTime(now),
		UpdatedAt:    timestamptzFromTime(now),
	})
	if err != nil {
		return Assistant{}, err
	}
	return agentFromInsertRow(row), nil
}

func (s *Store) UpdateAssistant(ctx context.Context, id string, name, description, providerID, defaultModel *string, settings json.RawMessage) (Assistant, error) {
	current, err := s.GetAssistant(ctx, id)
	if err != nil {
		return Assistant{}, err
	}

	if name != nil {
		if strings.TrimSpace(*name) == "" {
			return Assistant{}, fmt.Errorf("agent name is required")
		}
		current.Name = *name
	}
	if description != nil {
		current.Description = *description
	}
	if providerID != nil {
		pid := *providerID
		current.InferenceConnectionID = &pid
	}
	if defaultModel != nil {
		model := *defaultModel
		current.DefaultModel = &model
	}
	if len(settings) > 0 {
		merged, err := MergeSettings(current.Settings, settings)
		if err != nil {
			return Assistant{}, err
		}
		current.Settings = merged
	}
	switch {
	case current.InferenceConnectionID == nil && current.DefaultModel == nil:
		// incomplete: skip provider/model validation
	case current.InferenceConnectionID == nil || current.DefaultModel == nil:
		return Assistant{}, fmt.Errorf("provider and model must be set together")
	default:
		if err := s.validateInferenceConnectionAndModel(ctx, *current.InferenceConnectionID, *current.DefaultModel); err != nil {
			return Assistant{}, err
		}
	}

	current.Version++
	now := time.Now().UTC()
	row, err := s.q.UpdateAssistant(ctx, db.UpdateAssistantParams{
		ID:           id,
		Name:         current.Name,
		Description:  current.Description,
		Version:      int32(current.Version),
		InferenceConnectionID:   current.InferenceConnectionID,
		DefaultModel: current.DefaultModel,
		Settings:     rawOrDefault(current.Settings, "{}"),
		UpdatedAt:    timestamptzFromTime(now),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Assistant{}, newAssistantNotFound(id)
		}
		return Assistant{}, err
	}
	return agentFromUpdateRow(row), nil
}

func (s *Store) DeleteAssistant(ctx context.Context, id string) error {
	if _, err := s.GetAssistant(ctx, id); err != nil {
		return err
	}
	n, err := s.CountThreadsByAssistant(ctx, id)
	if err != nil {
		return err
	}
	if n > 0 {
		return ErrAssistantInUse
	}
	if err := s.q.DeleteAssistant(ctx, id); err != nil {
		if isFKViolation(err) {
			return ErrAssistantInUse
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

func (s *Store) CountThreadsByAssistant(ctx context.Context, agentID string) (int64, error) {
	n, err := s.q.CountThreadsByAssistant(ctx, &agentID)
	if err != nil {
		return 0, fmt.Errorf("count threads by agent: %w", err)
	}
	return n, nil
}

func (s *Store) PinThreadAssistant(ctx context.Context, threadID, agentID string) error {
	return s.inTx(ctx, func(q *db.Queries) error {
		row, err := q.GetThreadForUpdate(ctx, threadID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return newThreadNotFound(threadID)
			}
			return fmt.Errorf("get thread: %w", err)
		}
		if row.AssistantID != nil {
			if *row.AssistantID == agentID {
				return nil
			}
			return ErrAssistantLocked
		}
		if _, err := q.PinThreadAssistant(ctx, db.PinThreadAssistantParams{
			ID:        threadID,
			AssistantID:   &agentID,
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
		if assistant.CaptureSessionID != "" {
			if err := q.LinkHopCapturesToMessage(ctx, db.LinkHopCapturesToMessageParams{
				ThreadID:  threadID,
				SessionID: &assistant.CaptureSessionID,
				MessageID: &assistantID,
			}); err != nil {
				return fmt.Errorf("link hop captures: %w", err)
			}
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
		AssistantID:  agentID,
		CurrentModel: currentModel,
		ViewModeID:   viewModeID,
		ProjectID:    projectID,
		CreatedAt:    timeFromTimestamptz(createdAt),
		UpdatedAt:    timeFromTimestamptz(updatedAt),
	}
}

func threadFromRow(row db.GetThreadRow) Thread {
	return threadFromFields(row.ID, row.Title, row.TitleSource, row.AssistantID, row.CurrentModel, row.ViewModeID, row.ProjectID, row.CreatedAt, row.UpdatedAt)
}

func threadFromInsertRow(row db.InsertThreadRow) Thread {
	return threadFromFields(row.ID, row.Title, row.TitleSource, row.AssistantID, row.CurrentModel, row.ViewModeID, row.ProjectID, row.CreatedAt, row.UpdatedAt)
}

func threadFromRenameRow(row db.RenameThreadRow) Thread {
	return threadFromFields(row.ID, row.Title, row.TitleSource, row.AssistantID, row.CurrentModel, row.ViewModeID, row.ProjectID, row.CreatedAt, row.UpdatedAt)
}

func threadFromSetViewModeRow(row db.SetThreadViewModeRow) Thread {
	return threadFromFields(row.ID, row.Title, row.TitleSource, row.AssistantID, row.CurrentModel, row.ViewModeID, row.ProjectID, row.CreatedAt, row.UpdatedAt)
}

func threadFromListRow(row db.ListThreadsRow) Thread {
	return threadFromFields(row.ID, row.Title, row.TitleSource, row.AssistantID, row.CurrentModel, row.ViewModeID, row.ProjectID, row.CreatedAt, row.UpdatedAt)
}

func (s *Store) validateInferenceConnectionAndModel(ctx context.Context, providerID, defaultModel string) error {
	p, err := s.GetInferenceConnection(ctx, providerID)
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

	agents, err := q.ListAssistantsByInferenceConnection(ctx, &providerID)
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

func providerFromDB(row db.InferenceConnection) (InferenceConnection, error) {
	models, err := unmarshalModels(row.Models)
	if err != nil {
		return InferenceConnection{}, err
	}
	return InferenceConnection{
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

func agentFromInsertRow(row db.InsertAssistantRow) Assistant {
	return agentFromJoined(
		row.ID,
		row.Name,
		row.Description,
		row.Version,
		row.InferenceConnectionID,
		row.DefaultModel,
		nil,
		row.Settings,
		row.CreatedAt,
		row.UpdatedAt,
	)
}

func agentFromUpdateRow(row db.UpdateAssistantRow) Assistant {
	return agentFromJoined(
		row.ID,
		row.Name,
		row.Description,
		row.Version,
		row.InferenceConnectionID,
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
	providerID, defaultModel, inferenceConnectionName *string,
	settings []byte,
	createdAt, updatedAt pgtype.Timestamptz,
) Assistant {
	return Assistant{
		ID:           id,
		Name:         name,
		Description:  description,
		Version:      int(version),
		InferenceConnectionID:   providerID,
		InferenceConnectionName: inferenceConnectionName,
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
	switch typ {
	case TypeOpenAICompatible, TypeOpenCodeZen, TypeOpenCodeGo, TypeUnslothStudio:
		return true
	default:
		return false
	}
}

func newID(prefix string) (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(b), nil
}
