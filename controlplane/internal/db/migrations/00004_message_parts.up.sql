-- +goose Up
ALTER TABLE messages
  ADD COLUMN parts jsonb NOT NULL DEFAULT '[]'::jsonb,
  ADD COLUMN model text,
  ADD COLUMN provider_id text,
  ADD COLUMN provider_name text,
  ADD COLUMN stop_reason text;
