package modelspecs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/tryy3/agent-fabric/internal/db"
)

// Service owns the specs snapshot: it syncs from the configured source,
// persists the document and serves an in-memory index.
type Service struct {
	q      *db.Queries
	client *http.Client
	now    func() time.Time

	syncMu sync.Mutex // serializes Sync calls

	mu        sync.RWMutex
	providers map[string]Provider
	logos     map[string]logo
	row       db.ModelSpec
}

// New loads the stored snapshot. A nil client uses a 60s-timeout default.
func New(ctx context.Context, q *db.Queries, client *http.Client) (*Service, error) {
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	s := &Service{q: q, client: client, now: time.Now}
	if err := s.reload(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Service) reload(ctx context.Context) error {
	row, err := s.q.GetModelSpecs(ctx)
	if err != nil {
		return fmt.Errorf("load model specs: %w", err)
	}
	providers, err := decodeProviders(row.Data)
	if err != nil {
		// A corrupt snapshot must not stop boot; the next sync replaces it.
		slog.Warn("model specs snapshot unreadable", "err", err)
		providers = map[string]Provider{}
	}
	s.mu.Lock()
	s.row, s.providers, s.logos = row, providers, map[string]logo{}
	s.mu.Unlock()
	return nil
}

func decodeProviders(raw []byte) (map[string]Provider, error) {
	out := map[string]Provider{}
	if len(raw) == 0 {
		return out, nil
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	for pid, p := range out {
		if p.ID == "" {
			p.ID = pid
		}
		for mid, m := range p.Models {
			if m.ID == "" {
				m.ID = mid
				p.Models[mid] = m
			}
		}
		out[pid] = p
	}
	return out, nil
}

// Status returns settings, snapshot counts and last-attempt state.
func (s *Service) Status() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	st := Status{
		Settings:          settingsOf(s.row),
		SnapshotSourceURL: s.row.SnapshotSourceUrl,
		ProviderCount:     len(s.providers),
		Bytes:             s.row.Bytes,
		LastSyncedAt:      tsPtr(s.row.FetchedAt),
		LastAttemptAt:     tsPtr(s.row.LastAttemptAt),
		LastError:         s.row.LastError,
	}
	st.EffectiveSourceURL = st.Settings.EffectiveSourceURL()
	for _, p := range s.providers {
		st.ModelCount += len(p.Models)
	}
	return st
}

func settingsOf(r db.ModelSpec) Settings {
	return Settings{SourceURL: r.SourceUrl, SyncIntervalHours: int(r.SyncIntervalHours), Enabled: r.Enabled}
}

func tsPtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

// Providers lists providers sorted by id.
func (s *Service) Providers() []Provider {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Provider, 0, len(s.providers))
	for _, p := range s.providers {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Provider returns one provider by id.
func (s *Service) Provider(id string) (Provider, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.providers[id]
	return p, ok
}

// UpdateSettings validates and stores new settings.
func (s *Service) UpdateSettings(ctx context.Context, in Settings) (Settings, error) {
	if in.SyncIntervalHours <= 0 {
		in.SyncIntervalHours = DefaultSyncIntervalHours
	}
	if in.SourceURL != "" {
		u, err := url.Parse(in.SourceURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return Settings{}, fmt.Errorf("sourceUrl must be an http(s) URL")
		}
	}
	err := s.q.UpdateModelSpecsSettings(ctx, db.UpdateModelSpecsSettingsParams{
		SourceUrl: in.SourceURL, SyncIntervalHours: int32(in.SyncIntervalHours), Enabled: in.Enabled,
	})
	if err != nil {
		return Settings{}, fmt.Errorf("update model specs settings: %w", err)
	}
	if err := s.reload(ctx); err != nil {
		return Settings{}, err
	}
	return in, nil
}

// Sync downloads the configured source and replaces the snapshot. On any
// failure the previous snapshot stays in place and the error is recorded.
// An unchanged ETag (304) only refreshes the timestamp.
func (s *Service) Sync(ctx context.Context) (Status, error) {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()

	s.mu.RLock()
	cfg := settingsOf(s.row)
	prevETag, prevSource := s.row.Etag, s.row.SnapshotSourceUrl
	s.mu.RUnlock()
	source := cfg.EffectiveSourceURL()

	err := s.sync(ctx, source, prevETag, prevSource)
	if err != nil {
		_ = s.q.RecordModelSpecsError(ctx, db.RecordModelSpecsErrorParams{
			LastAttemptAt: ts(s.now()), LastError: err.Error(),
		})
	}
	if rerr := s.reload(ctx); rerr != nil && err == nil {
		err = rerr
	}
	return s.Status(), err
}

func (s *Service) sync(ctx context.Context, source, prevETag, prevSource string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return fmt.Errorf("create specs request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if prevETag != "" && prevSource == source {
		req.Header.Set("If-None-Match", prevETag)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("fetch specs: %w", err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusNotModified:
		return s.q.TouchModelSpecsSnapshot(ctx, ts(s.now()))
	case resp.StatusCode != http.StatusOK:
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("specs HTTP %d: %s", resp.StatusCode, bytes.TrimSpace(snippet))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxBodyBytes+1))
	if err != nil {
		return fmt.Errorf("read specs: %w", err)
	}
	if len(body) > MaxBodyBytes {
		return fmt.Errorf("specs document exceeds %d bytes", MaxBodyBytes)
	}
	providers, err := decodeProviders(body)
	if err != nil {
		return fmt.Errorf("decode specs: %w", err)
	}
	if len(providers) == 0 {
		return fmt.Errorf("specs document has no providers")
	}
	return s.q.SaveModelSpecsSnapshot(ctx, db.SaveModelSpecsSnapshotParams{
		SnapshotSourceUrl: source,
		Etag:              resp.Header.Get("ETag"),
		Bytes:             int64(len(body)),
		FetchedAt:         ts(s.now()),
		Data:              body,
	})
}

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

// due reports whether a sync should run now.
func (s *Service) due() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cfg := settingsOf(s.row)
	if !cfg.Enabled {
		return false
	}
	if !s.row.FetchedAt.Valid || s.row.SnapshotSourceUrl != cfg.EffectiveSourceURL() {
		// No usable snapshot: try now, then at most hourly while it keeps failing.
		return !s.row.LastAttemptAt.Valid || s.now().Sub(s.row.LastAttemptAt.Time) >= time.Hour
	}
	if s.row.LastAttemptAt.Valid && s.row.LastAttemptAt.Time.After(s.row.FetchedAt.Time) &&
		s.now().Sub(s.row.LastAttemptAt.Time) < time.Hour {
		// The last attempt failed: back off instead of retrying every tick.
		return false
	}
	interval := time.Duration(cfg.SyncIntervalHours) * time.Hour
	return s.now().Sub(s.row.FetchedAt.Time) >= interval
}

// Run syncs on boot when due, then re-checks every few minutes until ctx ends.
func (s *Service) Run(ctx context.Context) {
	check := func() {
		if !s.due() {
			return
		}
		if st, err := s.Sync(ctx); err != nil {
			slog.Warn("model specs sync failed", "source", st.EffectiveSourceURL, "err", err)
		} else {
			slog.Info("model specs synced", "source", st.EffectiveSourceURL,
				"providers", st.ProviderCount, "models", st.ModelCount)
		}
	}
	check()
	t := time.NewTicker(10 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			check()
		}
	}
}

// QueriesForTest exposes the queries handle so tests can build a second
// Service over the same database.
func (s *Service) QueriesForTest() *db.Queries { return s.q }
