-- +goose Up
ALTER TABLE plane_settings
    ADD COLUMN integrations jsonb NOT NULL DEFAULT '{}'::jsonb;
