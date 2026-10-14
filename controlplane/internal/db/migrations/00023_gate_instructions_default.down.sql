-- +goose Down
ALTER TABLE plane_settings
    ALTER COLUMN gate_instructions SET DEFAULT '';

UPDATE plane_settings
SET gate_instructions = ''
WHERE gate_instructions = $gate$# Gate instructions

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
