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

Use the `compose.sandbox.yaml` override to mount `/var/run/docker.sock` into `controlplane`. The controlplane image bundles the `docker` CLI; only the host socket is mounted.

Do **not** bind-mount the host’s `/usr/bin/docker` into the container — that commonly fails with `fork/exec ... no such file or directory` (symlink into paths that do not exist in the image, or a dynamically linked binary without its libs).

```bash
# GID that owns the socket on the host:
stat -c '%g %n' /var/run/docker.sock
echo "DOCKER_GID=$(stat -c '%g' /var/run/docker.sock)" >> .env

# Must include BOTH compose files, then recreate controlplane:
docker compose --env-file .env -f compose.yaml -f compose.sandbox.yaml up -d --force-recreate controlplane

# Sanity check (need Client AND Server):
docker compose exec controlplane id
docker compose exec controlplane ls -la /var/run/docker.sock
docker compose exec controlplane docker version
```

`compose.sandbox.yaml` uses `privileged: true`, `userns_mode: host`, and `group_add: [$DOCKER_GID]`. Mounting the Docker socket is already root-equivalent on the host.

If `docker version` still only shows Client + permission denied:

1. Confirm the sandbox file is applied:  
   `docker compose -f compose.yaml -f compose.sandbox.yaml config | grep -E 'privileged|userns|docker.sock'`
2. Confirm recreate actually happened (`--force-recreate controlplane`).
3. Rootless Docker: set `DOCKER_SOCK` to `$XDG_RUNTIME_DIR/docker.sock` (not `/var/run/docker.sock`).

## Local Postgres-only (dev)

The root [`docker-compose.yml`](../docker-compose.yml) still starts only Postgres for `go run` / `flutter run` against a local controlplane.
