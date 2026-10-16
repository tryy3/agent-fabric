#!/usr/bin/env python3
"""Narrow HTML→Markdown sidecar speaking a subset of Crawl4AI's API.

Implements `POST /md` for `raw:` URLs only (the control plane fetches the page
and sends its HTML), so the plane's Crawl4AI driver works against this stub and
a real Crawl4AI server alike. No remote crawl in V1.
"""

from __future__ import annotations

import json
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

from markdownify import markdownify as md


class Handler(BaseHTTPRequestHandler):
    def log_message(self, fmt: str, *args) -> None:  # quieter
        return

    def _json(self, code: int, payload: dict) -> None:
        body = json.dumps(payload).encode("utf-8")
        self.send_response(code)
        self.send_header("content-type", "application/json")
        self.send_header("content-length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self) -> None:  # noqa: N802
        if self.path == "/health":
            self._json(200, {"ok": True})
            return
        self._json(404, {"error": "not found"})

    def do_POST(self) -> None:  # noqa: N802
        if self.path != "/md":
            self._json(404, {"error": "not found"})
            return
        length = int(self.headers.get("content-length") or 0)
        if length > 6 * 1024 * 1024:
            self._json(400, {"error": "body too large"})
            return
        raw = self.rfile.read(length)
        try:
            body = json.loads(raw.decode("utf-8") or "{}")
        except json.JSONDecodeError as e:
            self._json(400, {"error": str(e)})
            return
        url = str(body.get("url") or "")
        if url.startswith("raw://"):
            html = url[len("raw://"):]
        elif url.startswith("raw:"):
            html = url[len("raw:"):]
        else:
            self._json(400, {"error": "only raw: URLs are supported"})
            return
        markdown = md(html, heading_style="ATX")
        self._json(
            200,
            {
                "url": url,
                "filter": body.get("f") or "fit",
                "markdown": markdown,
                "success": True,
            },
        )


if __name__ == "__main__":
    ThreadingHTTPServer(("0.0.0.0", 11235), Handler).serve_forever()
