-- +goose Up
-- Harness (plane) and Assistant instruction sources for composed effective instructions.

ALTER TABLE plane_settings
    ADD COLUMN harness_instructions text NOT NULL DEFAULT '';

ALTER TABLE assistants
    ADD COLUMN instructions text NOT NULL DEFAULT '';
