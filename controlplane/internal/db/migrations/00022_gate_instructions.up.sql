-- +goose Up
-- Gate instructions: the user's plain-language rules about what the agent may
-- and may not do. The plane-wide default lives on plane_settings; an assistant
-- value replaces it. NULL on an assistant means "use the plane default", an
-- empty string means "no gate instructions for this assistant".

ALTER TABLE plane_settings
    ADD COLUMN gate_instructions text NOT NULL DEFAULT '';

ALTER TABLE assistants
    ADD COLUMN gate_instructions text NULL;
