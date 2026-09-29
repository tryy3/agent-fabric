# Deploy with Docker

Images are published to GHCR on pushes to `main` and `v*` tags:

- `ghcr.io/<owner>/<repo>/controlplane`
- `ghcr.io/<owner>/<repo>/client-web`

Both are multi-arch (`linux/amd64`, `linux/arm64`).

## What you need on the server

- Docker Engine + Compose plugin
- `docker login ghcr.io` (if the packages are private)
- This `deploy/` directory (`compose.yaml`, `config.json`, optional `.env`)

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

# Optional: web search / page-read sidecars (SearXNG, get-md, Crawl4AI)
docker compose --env-file .env -f compose.yaml -f compose.web-integrations.yaml up -d
```

Open `http://localhost:8080` (or `HTTP_PORT` from `.env`).

Services:

| Service | Role |
| --- | --- |
| `postgres` | Catalog, threads, settings |
| `controlplane` | ACP `/acp` + catalog `/v1` |
| `client-web` | Flutter static + nginx proxy to the plane |
| `searxng` / `get-md` / `crawl4ai` | Optional web-integration sidecars (see `compose.web-integrations.yaml`) |

## Web integrations with a host-run control plane

When you run `go -C controlplane run …` and `flutter run` on the host (repo-root workflow), Docker DNS names like `http://searxng:8080` do not resolve. Publish sidecar ports and configure **external** endpoints:

```bash
cd deploy
docker compose --env-file .env \
  -f compose.yaml \
  -f compose.web-integrations.yaml \
  -f compose.web-integrations.local.yaml \
  up -d --build searxng get-md crawl4ai
```

| Service | Host URL (catalog `mode=external`) | Capabilities |
| --- | --- | --- |
| SearXNG | `http://127.0.0.1:8081` (`SEARXNG_HOST_PORT`) | `web_search` |
| get-md | `http://127.0.0.1:3000` (`GET_MD_HOST_PORT`) | `fetch_page` |
| Crawl4AI | `http://127.0.0.1:11235` (`CRAWL4AI_HOST_PORT`) | `fetch_page` |
| Linkup (hosted, no sidecar) | API key only (endpoint defaults to `https://mcp.linkup.so/mcp`) | `web_search` + `fetch_page` |

In Settings → Integrations, create each kind with **external** mode and the localhost URL above (do not use bundled mode for a host-run plane). Set plane defaults, then start a **new** ACP session so the pin picks them up.

Postgres for that workflow is still typically the root `docker compose up -d` (port `5432`) or the `postgres` service from `deploy/compose.yaml`.

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
| `PLANE_CONFIG` | Path to host engine config (default `./config.json`) |
| `DOCKER_SOCK` | Host Docker socket for agent sandboxes |
| `CATALOG_BASE` / `ACP_URI` | Optional Flutter URL overrides |
| `CONTROLPLANE_UPSTREAM` | Set in compose; nginx → plane on the Docker network |

## Agent sandboxes

Use the `compose.sandbox.yaml` override to mount a Docker socket into `controlplane`. The image bundles the `docker` CLI; only the socket is mounted (never the host `/usr/bin/docker` binary).

### Rootless Docker (`nobody:nobody` on the sock)

If **host** shows `root docker` but **inside the container** the sock is `nobody nobody`, you are almost certainly running **rootless** Docker (`dockerd-rootless`). Container “root” is not host root, so `/var/run/docker.sock` is unreachable.

```bash
docker info | grep -i rootless
ls -la "$XDG_RUNTIME_DIR/docker.sock"

# Point compose at the rootless socket (owned by your user):
export DOCKER_SOCK="$XDG_RUNTIME_DIR/docker.sock"
echo "DOCKER_SOCK=$DOCKER_SOCK" >> .env
echo "DOCKER_GID=$(stat -c '%g' "$DOCKER_SOCK")" >> .env

docker compose --env-file .env -f compose.yaml -f compose.sandbox.yaml \
  up -d --force-recreate controlplane

docker compose exec controlplane ls -la /var/run/docker.sock   # should NOT be nobody:nobody
docker compose exec controlplane docker version               # Client + Server
```

Alternatively, switch the host back to **rootful** Docker (`sudo systemctl enable --now docker` and stop rootless) if you want `/var/run/docker.sock`.

### Rootful Docker

```bash
echo "DOCKER_SOCK=/var/run/docker.sock" >> .env
echo "DOCKER_GID=$(stat -c '%g' /var/run/docker.sock)" >> .env

docker compose --env-file .env -f compose.yaml -f compose.sandbox.yaml \
  up -d --force-recreate controlplane

docker compose exec controlplane docker version
```

`compose.sandbox.yaml` also sets `privileged: true` and `userns_mode: host` for rootful installs.

## Local Postgres-only (dev)

The root [`docker-compose.yml`](../docker-compose.yml) still starts only Postgres for `go run` / `flutter run` against a local controlplane.
