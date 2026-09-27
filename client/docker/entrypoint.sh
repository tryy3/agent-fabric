#!/bin/sh
set -eu

CONFIG_PATH=/usr/share/nginx/html/config.json

# Runtime Flutter endpoints (empty → same-origin in the app).
catalog_base=${CATALOG_BASE:-}
acp_uri=${ACP_URI:-}

# Minimal JSON; values are shell-escaped for quotes/backslashes only.
json_escape() {
  printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'
}

cat >"$CONFIG_PATH" <<EOF
{
  "catalogBase": "$(json_escape "$catalog_base")",
  "acpUri": "$(json_escape "$acp_uri")"
}
EOF

export CONTROLPLANE_UPSTREAM="${CONTROLPLANE_UPSTREAM:-http://controlplane:8080}"

exec /docker-entrypoint.sh nginx -g 'daemon off;'
