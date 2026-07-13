# ptpatronus-plugin-sdk

Python SDK + the `pluginctl` dev tool for **PTPatronus external-http plugins**.

PTPatronus is closed-source — but you don't need the host source or binary to develop plugins.
This package is the public developer surface: an importable Host API client + the `ptp-pluginctl`
command (scaffold/validate/test/package/publish plugins).

- Install: `pip install ptpatronus-plugin-sdk`
- CLI: `ptp-pluginctl --help` (create / validate / dev / run / package / market ...)
- SDK: `from ptpatronus_plugin_sdk import PTPHostClient`

Full developer guide: `DEVGUIDE.md` (shipped alongside, also at https://ptang.top/devkit/devguide).

## Write a plugin (Python, stdlib)

```python
# plugin.py — a minimal external-http plugin using the SDK
import json, os
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from ptpatronus_plugin_sdk import PTPHostClient

PLUGIN_TOKEN = os.environ.get("PTP_PLUGIN_TOKEN", "")

class Handler(BaseHTTPRequestHandler):
    def _json(self, s, d):
        raw = json.dumps(d).encode(); self.send_response(s)
        self.send_header("Content-Type", "application/json"); self.send_header("Content-Length", str(len(raw)))
        self.end_headers(); self.wfile.write(raw)
    def _body(self):
        n = int(self.headers.get("Content-Length", "0") or "0")
        return json.loads(self.rfile.read(n).decode()) if n else {}
    def _auth(self):
        return not PLUGIN_TOKEN or self.headers.get("Authorization") == f"Bearer {PLUGIN_TOKEN}"
    def do_GET(self):
        if self.path == "/health": self._json(200, {"ok": True})
        else: self._json(404, {"error": "not found"})
    def do_POST(self):
        if not self._auth(): self._json(401, {"error": "unauthorized"}); return
        p = self._body()
        if self.path == "/action":
            host = PTPHostClient.from_payload(p)        # host context from the request
            cfg = p.get("config") or {}
            if p.get("action") == "ping":
                if host: host.log("info", "ping", "pong")
                self._json(200, {"ok": True, "output": {"msg": cfg.get("prefix", "pong")}})
            else:
                self._json(200, {"ok": False, "error": "unknown action"})
        else:
            self._json(404, {"error": "not found"})
    def log_message(self, *a): pass

if __name__ == "__main__":
    ThreadingHTTPServer(("127.0.0.1", int(os.environ.get("PTP_PLUGIN_PORT", "19090"))), Handler).serve_forever()
```

Scaffold + test it without the closed binary:

```bash
ptp-pluginctl create my-plugin --directory my-plugin
ptp-pluginctl mock-host          # fake Host API for local dev
ptp-pluginctl run my-plugin --action ping --once
ptp-pluginctl package my-plugin --json-out dist/plugin-package.json
```

## API surface (PTPHostClient)

`from_env()` / `from_payload(payload)` · `has_permission(p)` · `has_capability(c)` ·
`config()` · `set_runtime_config(k,v)` · `log(level,event,msg,data)` · `notice(title,body,level)` ·
`publish(type,data)` · `kv_get/set/delete(key)` · `sites()` · `site_cookie(id)` · `media_search(keyword,year)` ·
`capabilities()`. All synchronous; raise `HostError` on non-2xx.

### Tool RPC — ~80 host capabilities via one entry point

`tools()` lists every tool your permissions allow; `call(name, input)` invokes any of them. Covers
downloader, media server, subscribe, cloud/STRM, brush-flow, RSS, subtitle, reseed, transfer,
dashboard, notification, filter, file storage, workflow. Typed convenience wrappers exist for the
hottest domains (`torrents_list`, `torrents_add`, `mediaserver_library_scan`, `subscribe_list`,
`send_notification`, `dashboard_overview`, …). Declare domain permissions (`downloader:read`,
`mediaserver:write`, …) in your manifest. Full catalog + per-tool input schemas: `DEVGUIDE.md` →
"Tool RPC".

```python
host.mediaserver_library_scan(server_id=1)        # typed convenience
host.call("subscribe_check", {"id": 7})            # generic — works for any tool
for t in host.tools(): print(t["name"])            # self-describing discovery
```

## Versioning

Package version (`0.1.0`) is independent of the PTPatronus app. The plugin **contract** is
`api_version: "1"`; breaking contract changes bump both.

License: MIT.
