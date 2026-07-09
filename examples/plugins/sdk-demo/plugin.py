"""SDK Demo plugin — uses ptpatronus-plugin-sdk (optional import, falls back to stdlib).

Demonstrates the public Python SDK: PTPHostClient.from_env() + log() + kv_get/set against the
Host API. In production the host `pip install`s `ptpatronus-plugin-sdk` (declared in requirements)
into the plugin's .deps; for local `pluginctl run` put the SDK on PYTHONPATH.
"""
import json
import os
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

try:
    from ptpatronus_plugin_sdk import PTPHostClient
    _HAS_SDK = True
except ImportError:  # graceful: works without the SDK too
    PTPHostClient = None
    _HAS_SDK = False

TOKEN = os.environ.get("PTP_PLUGIN_TOKEN", "")


class Handler(BaseHTTPRequestHandler):
    def _json(self, status, payload):
        raw = json.dumps(payload).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(raw)))
        self.end_headers()
        self.wfile.write(raw)

    def _body(self):
        n = int(self.headers.get("Content-Length", "0") or "0")
        return json.loads(self.rfile.read(n).decode("utf-8")) if n else {}

    def do_GET(self):
        if self.path == "/health":
            self._json(200, {"ok": True})
        else:
            self._json(404, {"error": "not found"})

    def do_POST(self):
        if TOKEN and self.headers.get("Authorization") != f"Bearer {TOKEN}":
            self._json(401, {"error": "unauthorized"})
            return
        payload = self._body()
        if self.path == "/action":
            try:
                cfg = payload.get("config") or {}
                if payload.get("action") != "ping":
                    self._json(200, {"ok": False, "error": "unknown action"})
                    return
                if not _HAS_SDK:
                    self._json(200, {"ok": True, "output": {"msg": cfg.get("prefix", "pong"), "sdk": False}})
                    return
                host = PTPHostClient.from_env()
                host.log("info", "sdk.demo", "ping via SDK", {"action": "ping"})
                cur = host.kv_get("sdk-count")
                nxt = int(cur.value or 0) + 1 if cur.found else 1
                host.kv_set("sdk-count", nxt)
                self._json(200, {"ok": True, "output": {"msg": cfg.get("prefix", "pong"), "count": nxt, "sdk": True}})
            except Exception as exc:
                self._json(200, {"ok": False, "error": str(exc)})
        else:
            self._json(404, {"error": "not found"})

    def log_message(self, *args):
        return


if __name__ == "__main__":
    ThreadingHTTPServer(("127.0.0.1", int(os.environ.get("PTP_PLUGIN_PORT", "19090"))), Handler).serve_forever()
