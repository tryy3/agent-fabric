# Deploy with Docker

Images are published to GHCR on pushes to `main` and `v*` tags:

- `ghcr.io/<owner>/<repo>/controlplane`
- `ghcr.io/<owner>/<repo>/client-web`

Both are multi-arch (`linux/amd64`, `linux/arm64`).

## Quick start

```bash
# From a machine that can pull private GHCR packages (if the repo is private):
echo "$GITHUB_TOKEN" | docker login ghcr.io -u USERNAME --password-stdin

cd /path/to/agent-fabric
docker compose -f deploy/compose.yaml up -d
```

Open `http://localhost:8080`. The client-web container serves the Flutter app and reverse-proxies `/v1/` and `/acp` to `controlplane`.

Override image tags/names when needed:

```bash
export CONTROLPLANE_IMAGE=ghcr.io/tryy3/agent-fabric/controlplane:main
export CLIENT_WEB_IMAGE=ghcr.io/tryy3/agent-fabric/client-web:main
docker compose -f deploy/compose.yaml up -d
```

## Client URL env vars

| Variable | Where | Purpose |
| --- | --- | --- |
| `CONTROLPLANE_UPSTREAM` | client-web | Docker-network URL nginx uses to reach the plane (default `http://controlplane:8080`) |
| `CATALOG_BASE` | client-web | Optional catalog origin written to `/config.json` |
| `ACP_URI` | client-web | Optional ACP WebSocket URI written to `/config.json` |

If `CATALOG_BASE` and `ACP_URI` are unset (the compose default), the Flutter app uses **same-origin** from the browser address bar. That works with Tailscale Serve in front of port `8080` without rebuilding the image.

Example Tailscale:

```bash
sudo tailscale serve --bg http://127.0.0.1:8080
# Browse https://<machine>.<tailnet>.ts.net — leave CATALOG_BASE/ACP_URI unset
```

To pin different public URLs without same-origin:

```yaml
environment:
  CATALOG_BASE: https://agents.example.com
  ACP_URI: wss://agents.example.com/acp
```

## Controlplane

| Variable / file | Purpose |
| --- | --- |
| `DATABASE_URL` | Postgres DSN (overrides `sandbox.json`) |
| `deploy/sandbox.json` | Host engine config (`listenAddr`, `dataDir`, docker runtime) |

Mount `/var/run/docker.sock` into `controlplane` if agents need Docker sandboxes.

## Local Postgres-only (dev)

The root [`docker-compose.yml`](../docker-compose.yml) still starts only Postgres for `go run` / `flutter run` against a local controlplane.
