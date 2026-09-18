-- +goose Up
ALTER TABLE threads
  ADD COLUMN view_mode_id text NULL;
