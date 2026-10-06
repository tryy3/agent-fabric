-- +goose Up
CREATE TABLE model_specs (
    id TEXT PRIMARY KEY CHECK (id = 'default'),
    -- Settings: where to sync from ('' = default models.dev), how often, on/off.
    source_url TEXT NOT NULL DEFAULT '',
    sync_interval_hours INTEGER NOT NULL DEFAULT 24,
    enabled BOOLEAN NOT NULL DEFAULT true,
    -- Snapshot of the last successful sync plus status of the last attempt.
    snapshot_source_url TEXT NOT NULL DEFAULT '',
    etag TEXT NOT NULL DEFAULT '',
    bytes BIGINT NOT NULL DEFAULT 0,
    fetched_at TIMESTAMPTZ,
    last_attempt_at TIMESTAMPTZ,
    last_error TEXT NOT NULL DEFAULT '',
    data JSONB NOT NULL DEFAULT '{}'::jsonb
);

INSERT INTO model_specs (id) VALUES ('default');
