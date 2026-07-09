import json
import os
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib import request


PLUGIN_TOKEN = os.environ.get("PTP_PLUGIN_TOKEN", "")


def host_context(payload):
    host = payload.get("host") or {}
    return {
        "base_url": os.environ.get("PTP_HOST_URL") or host.get("base_url", ""),
        "token": os.environ.get("PTP_HOST_TOKEN") or host.get("token", ""),
    }


def host_request(host, method, path, body=None):
    if not host["base_url"] or not host["token"]:
        return None
    data = None
    headers = {"Authorization": f"Bearer {host['token']}"}
    if body is not None:
        data = json.dumps(body).encode("utf-8")
        headers["Content-Type"] = "application/json"
    req = request.Request(host["base_url"].rstrip("/") + path, data=data, headers=headers, method=method)
    with request.urlopen(req, timeout=5) as res:
        raw = res.read().decode("utf-8")
        return json.loads(raw) if raw else {}


class Handler(BaseHTTPRequestHandler):
    def _auth(self):
        return not PLUGIN_TOKEN or self.headers.get("Authorization") == f"Bearer {PLUGIN_TOKEN}"

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
        host = host_context(payload)
        if self.path == "/event":
            host_request(host, "POST", "/log", {
                "level": "info",
                "event": payload.get("type", "plugin.event"),
                "message": "external demo received event",
                "data": payload.get("data") or {},
            })
            self._json(200, {"ok": True})
            return
        if self.path == "/action":
            cfg = payload.get("config") or {}
            prefix = cfg.get("prefix") or "pong"
            counter = host_request(host, "GET", "/kv/ping-count") or {"found": False}
            next_count = int(counter.get("value") or 0) + 1
            host_request(host, "PUT", "/kv/ping-count", {"value": next_count})
            host_request(host, "POST", "/log", {
                "level": "info",
                "event": "demo.ping",
                "message": "ping action executed",
                "data": {"count": next_count},
            })
            host_request(host, "POST", "/events", {
                "type": "plugin.demo.ping",
                "data": {"count": next_count},
            })
            self._json(200, {
                "ok": True,
                "output": {
                    "message": f"{prefix}: {payload.get('input') or {}}",
                    "count": next_count,
                    "plugin_id": os.environ.get("PTP_PLUGIN_ID", ""),
                },
            })
            return
        self._json(404, {"error": "not found"})

    def log_message(self, fmt, *args):
        return


if __name__ == "__main__":
    port = int(os.environ.get("PTP_PLUGIN_PORT", "19090"))
    ThreadingHTTPServer(("127.0.0.1", port), Handler).serve_forever()
