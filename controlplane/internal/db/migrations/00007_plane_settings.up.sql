-- +goose Up
CREATE TABLE plane_settings (
    id         text PRIMARY KEY CHECK (id = 'default'),
    sandbox    jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);

ALTER TABLE agents
    ADD COLUMN settings jsonb NOT NULL DEFAULT '{}'::jsonb;
