#!/usr/bin/env bash
# Optional smoke checks for compose.web-integrations.yaml sidecars.
# Run from a host with curl after:
#   docker compose --env-file .env -f compose.yaml -f compose.web-integrations.yaml up -d --build
#
# Network access from the host requires temporarily publishing ports or
# `docker compose exec controlplane curl …` (preferred).

set -euo pipefail

run_in_plane() {
  docker compose -f compose.yaml -f compose.web-integrations.yaml exec -T controlplane "$@"
}

echo "== searxng health (json search) =="
run_in_plane curl -fsS 'http://searxng:8080/search?q=agent-fabric&format=json' | head -c 200
echo

echo "== get-md health =="
run_in_plane curl -fsS 'http://get-md:3000/health'
echo

echo "== get-md convert =="
run_in_plane curl -fsS -X POST 'http://get-md:3000/convert' \
  -H 'content-type: application/json' \
  -d '{"url":"https://example.com","contentType":"text/html","html":"<h1>Hi</h1><p>Hello</p>"}'
echo

echo "== crawl4ai health =="
run_in_plane curl -fsS 'http://crawl4ai:11235/health'
echo

echo "== crawl4ai convert =="
run_in_plane curl -fsS -X POST 'http://crawl4ai:11235/convert' \
  -H 'content-type: application/json' \
  -d '{"url":"https://example.com","contentType":"text/html","html":"<h1>Hi</h1><p>Hello</p>"}'
echo

echo "OK"
