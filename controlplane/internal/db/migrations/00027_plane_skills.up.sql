-- +goose Up
ALTER TABLE plane_settings
    ADD COLUMN skills jsonb NOT NULL DEFAULT '{}'::jsonb;
