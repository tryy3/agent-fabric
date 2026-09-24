-- +goose Up
CREATE TABLE resources (
    id         text PRIMARY KEY,
    name       text NOT NULL,
    kind       text NOT NULL CHECK (kind IN ('container')),
    spec       jsonb NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);

ALTER TABLE plane_settings
    ADD COLUMN environment jsonb NOT NULL DEFAULT '{}'::jsonb;
