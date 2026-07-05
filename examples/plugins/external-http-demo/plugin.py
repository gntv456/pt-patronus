import json
import os
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


TOKEN = os.environ.get("PTP_PLUGIN_TOKEN", "")


class Handler(BaseHTTPRequestHandler):
    def _auth(self):
        if not TOKEN:
            return True
        return self.headers.get("Authorization") == f"Bearer {TOKEN}"

    def _json(self, status, data):
        raw = json.dumps(data, ensure_ascii=False).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(raw)))
        self.end_headers()
        self.wfile.write(raw)

    def _body(self):
        length = int(self.headers.get("Content-Length", "0") or "0")
        if length <= 0:
            return {}
        return json.loads(self.rfile.read(length).decode("utf-8"))

    def do_GET(self):
        if self.path == "/health":
            self._json(200, {"ok": True})
            return
        self._json(404, {"error": "not found"})

    def do_POST(self):
        if not self._auth():
            self._json(401, {"error": "unauthorized"})
            return
        payload = self._body()
        if self.path == "/event":
            print("event:", json.dumps(payload, ensure_ascii=False), flush=True)
            self._json(200, {"ok": True})
            return
        if self.path == "/action":
            cfg = payload.get("config") or {}
            prefix = cfg.get("prefix") or "pong"
            self._json(200, {
                "ok": True,
                "output": {
                    "message": f"{prefix}: {payload.get('input', {})}",
                    "plugin_id": os.environ.get("PTP_PLUGIN_ID", "")
                }
            })
            return
        self._json(404, {"error": "not found"})

    def log_message(self, fmt, *args):
        return


if __name__ == "__main__":
    port = int(os.environ.get("PTP_PLUGIN_PORT", "19090"))
    ThreadingHTTPServer(("127.0.0.1", port), Handler).serve_forever()
