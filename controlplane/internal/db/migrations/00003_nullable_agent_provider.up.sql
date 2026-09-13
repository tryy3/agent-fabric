-- +goose Up
ALTER TABLE agents
    ALTER COLUMN provider_id DROP NOT NULL,
    ALTER COLUMN default_model DROP NOT NULL;
