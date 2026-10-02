-- +goose Up
-- Platform, runtime context, and Assistant instruction sources for composed
-- effective instructions.

ALTER TABLE plane_settings
    ADD COLUMN platform_instructions text NOT NULL DEFAULT '',
    ADD COLUMN runtime_context text NOT NULL DEFAULT '';

ALTER TABLE assistants
    ADD COLUMN instructions text NOT NULL DEFAULT '';
