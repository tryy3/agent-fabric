# Settings (plane-wide)

## Sub-features
`GET/PATCH /v1/settings` (environment image/kind, permissions defaults), `GET /v1/permissions/builtins`.

## How to get to it (user POV)
Flutter **Settings → Environment / Permissions**.

## Driving it with curl
`curl -s localhost:8099/v1/settings`, PATCH a field, GET again. Proof: GET reflects the PATCH; applies on the next prompt.

## Gotchas
Not yet driven end to end by this skill. Engine knobs (`listenAddr`, `databaseUrl`) live only in `config.json`, not here.
