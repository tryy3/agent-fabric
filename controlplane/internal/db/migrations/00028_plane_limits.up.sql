-- +goose Up
ALTER TABLE plane_settings
    ADD COLUMN limits jsonb NOT NULL DEFAULT '{}'::jsonb;
