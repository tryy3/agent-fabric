-- +goose Up
-- A new plane starts with default gate instructions, adapted from the
-- guardrails example in berget-ai/guardrails-md (MIT). The column default seeds
-- the lazily created plane_settings row. An empty string still means "no gate
-- instructions", so a user who clears the text keeps it empty.
--
-- The UPDATE is safe because gate_instructions exists only since 00022, which
-- has not shipped on main. An empty value on an existing row can only mean
-- "never set".

ALTER TABLE plane_settings
    ALTER COLUMN gate_instructions SET DEFAULT $gate$# Gate instructions

## The agent MUST NOT

- Change anything in production. Production changes go through Git and CD.
- Push directly to the main branch. All changes go through a pull request.
- Install software outside the project's declared dependencies.
- Send data to external services the user has not approved.
- Run irreversible operations against shared systems, such as deletions, cleanup and resets.

## The agent MAY

- Inspect any environment read-only.
- Run tests, lint and builds locally.
- Create branches and push feature branches.
- Read documentation from public sources.$gate$;

UPDATE plane_settings
SET gate_instructions = $gate$# Gate instructions

## The agent MUST NOT

- Change anything in production. Production changes go through Git and CD.
- Push directly to the main branch. All changes go through a pull request.
- Install software outside the project's declared dependencies.
- Send data to external services the user has not approved.
- Run irreversible operations against shared systems, such as deletions, cleanup and resets.

## The agent MAY

- Inspect any environment read-only.
- Run tests, lint and builds locally.
- Create branches and push feature branches.
- Read documentation from public sources.$gate$
WHERE gate_instructions = '';
