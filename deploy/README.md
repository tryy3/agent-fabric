# Deploy with Docker

Images are published to GHCR on pushes to `main` and `v*` tags:

- `ghcr.io/<owner>/<repo>/controlplane`
- `ghcr.io/<owner>/<repo>/client-web`

Both are multi-arch (`linux/amd64`, `linux/arm64`).

## What you need on the server

- Docker Engine + Compose plugin
- `docker login ghcr.io` (if the packages are private)
- This `deploy/` directory (`compose.yaml`, `sandbox.json`, optional `.env`)

No Go or Flutter SDK on the server.

## Quick start

```bash
cd deploy   # or use -f deploy/compose.yaml from the repo root
cp .env.example .env
# edit .env — change POSTGRES_PASSWORD; set image tags to :main or :latest

docker login ghcr.io
docker compose --env-file .env up -d

# Optional: agent Docker sandboxes on the host
docker compose --env-file .env -f compose.yaml -f compose.sandbox.yaml up -d
```

Open `http://localhost:8080` (or `HTTP_PORT` from `.env`).

Services:

| Service | Role |
| --- | --- |
| `postgres` | Catalog, threads, settings |
| `controlplane` | ACP `/acp` + catalog `/v1` |
| `client-web` | Flutter static + nginx proxy to the plane |

## Tailscale HTTPS

```bash
sudo tailscale serve --bg http://127.0.0.1:8080
```

Leave `CATALOG_BASE` / `ACP_URI` empty so the app uses same-origin from the browser URL.

## Environment reference

See [`.env.example`](.env.example).

| Variable | Purpose |
| --- | --- |
| `CONTROLPLANE_IMAGE` / `CLIENT_WEB_IMAGE` | GHCR image refs |
| `HTTP_PORT` | Host port for the UI (default `8080`) |
| `POSTGRES_*` | DB credentials (also baked into `DATABASE_URL`) |
| `SANDBOX_JSON` | Path to host engine config (default `./sandbox.json`) |
| `DOCKER_SOCK` | Host Docker socket for agent sandboxes |
| `CATALOG_BASE` / `ACP_URI` | Optional Flutter URL overrides |
| `CONTROLPLANE_UPSTREAM` | Set in compose; nginx → plane on the Docker network |

## Agent sandboxes

Use the `compose.sandbox.yaml` override to mount `/var/run/docker.sock` into `controlplane` so catalog sandbox kind `docker` can start containers on the host.

## Local Postgres-only (dev)

The root [`docker-compose.yml`](../docker-compose.yml) still starts only Postgres for `go run` / `flutter run` against a local controlplane.
