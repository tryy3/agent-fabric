#!/usr/bin/env python3
"""Narrow HTML→Markdown conversion sidecar (no remote crawl in V1)."""

from __future__ import annotations

import json
import re
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

from markdownify import markdownify as md


def title_from_html(html: str) -> str:
    m = re.search(r"<title[^>]*>([^<]*)</title>", html, re.I)
    return m.group(1).strip() if m else ""


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
        if self.path != "/convert":
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
        html = str(body.get("html") or "")
        url = str(body.get("url") or "")
        markdown = md(html, heading_style="ATX")
        self._json(
            200,
            {
                "title": title_from_html(html),
                "markdown": markdown,
                "url": url,
            },
        )


if __name__ == "__main__":
    ThreadingHTTPServer(("0.0.0.0", 11235), Handler).serve_forever()
