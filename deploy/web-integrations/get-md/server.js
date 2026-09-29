import http from "node:http";
import TurndownService from "turndown";

const port = Number(process.env.PORT || 3000);
const turndown = new TurndownService({ headingStyle: "atx", codeBlockStyle: "fenced" });

function readBody(req) {
  return new Promise((resolve, reject) => {
    const chunks = [];
    let size = 0;
    req.on("data", (c) => {
      size += c.length;
      if (size > 6 * 1024 * 1024) {
        reject(new Error("body too large"));
        req.destroy();
        return;
      }
      chunks.push(c);
    });
    req.on("end", () => resolve(Buffer.concat(chunks).toString("utf8")));
    req.on("error", reject);
  });
}

function titleFromHtml(html) {
  const m = html.match(/<title[^>]*>([^<]*)<\/title>/i);
  return m ? m[1].trim() : "";
}

const server = http.createServer(async (req, res) => {
  try {
    if (req.method === "GET" && req.url === "/health") {
      res.writeHead(200, { "content-type": "application/json" });
      res.end(JSON.stringify({ ok: true }));
      return;
    }
    if (req.method === "POST" && req.url === "/convert") {
      const raw = await readBody(req);
      const body = JSON.parse(raw || "{}");
      const html = String(body.html || "");
      const url = String(body.url || "");
      const markdown = turndown.turndown(html);
      const title = titleFromHtml(html);
      res.writeHead(200, { "content-type": "application/json" });
      res.end(JSON.stringify({ title, markdown, url }));
      return;
    }
    res.writeHead(404, { "content-type": "application/json" });
    res.end(JSON.stringify({ error: "not found" }));
  } catch (err) {
    res.writeHead(400, { "content-type": "application/json" });
    res.end(JSON.stringify({ error: String(err?.message || err) }));
  }
});

server.listen(port, "0.0.0.0");
