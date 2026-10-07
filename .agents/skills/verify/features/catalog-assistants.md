# Catalog: connections, models, assistants

## Sub-features
Inference connections (`/v1/inference/connections`), model refresh, assistants with `settings.inference` / `settings.permissions`.

## How to get to it (user POV)
Flutter **Settings → Connections / Assistants**, or curl on the catalog port.

## Driving it with curl
`seed.sh` does create → `POST .../models/refresh` → `POST /v1/assistants`. Then `curl -s localhost:8099/v1/assistants/$(cat /tmp/af-verify/run/assistant.id)`; PATCH `{"settings":{"inference":{"temperature":0.2}}}` and GET again.
Proof: the assistant GET returns the patched `settings.inference`; connection GET shows cached models containing `fake-model`.

## Gotchas
Models must be refreshed before assistant creation. `apiKey` is never returned in plain text — don't assert on it. Inference settings are pinned at `session/new`, so a PATCH only affects new sessions.
