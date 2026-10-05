-- +goose Up
ALTER TABLE plane_settings
    ADD COLUMN permissions jsonb NOT NULL DEFAULT '{}'::jsonb;
