# PTPatronus Plugin Development Guide

PTPatronus is **closed-source**, but the plugin platform is an open developer surface. You can build,
test, and publish external plugins **without ever seeing host source or running the closed binary** —
you target the stable **plugin contract** (`api_version: "1"`) using the public dev kit.

This guide ships inside `ptpatronus-plugin-sdk` (pip) and `@ptpatronus/plugin-sdk` (npm), and is the
canonical public reference. It contains no host internals.

## What a plugin is

A PTPatronus plugin is an **external HTTP process**. The host launches it (or proxies to a base URL),
calls `POST /action` and `POST /event` on it, and the plugin calls back into the **Host API** (a small
loopback HTTP surface) for things like logging, KV, notifications, site cookies, and media search.

- Runtime: `external-http`
- Process: a plain HTTP server you write (Python stdlib, Node/TypeScript, anything)
- Lifecycle: host health-checks `GET /health`, then dispatches actions/events; auto-restarts on crash

## Install the dev kit

```bash
pip install ptpatronus-plugin-sdk      # Python SDK + the `ptp-pluginctl` CLI
# or, for TypeScript plugins:
npm install @ptpatronus/plugin-sdk
```

`ptp-pluginctl` (the CLI) is the only tool you need: scaffold, validate, test locally, package, publish.

## Quick start (Python)

```bash
ptp-pluginctl create my-plugin --directory my-plugin          # scaffold (+ optionally --vendor-tools to pin the tool into your repo)
cd my-plugin
ptp-pluginctl validate .                                       # check plugin.json
ptp-pluginctl mock-host --port 19091 --token dev-token &       # fake Host API — no closed binary needed
ptp-pluginctl run . --action ping --once                       # launch plugin + invoke an action against the mock host
ptp-pluginctl package . --json-out dist/plugin-package.json    # build zip + metadata
```

A minimal plugin process (Python, using the SDK):

```python
import json, os
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from ptpatronus_plugin_sdk import PTPHostClient

TOKEN = os.environ.get("PTP_PLUGIN_TOKEN", "")

class H(BaseHTTPRequestHandler):
    def _j(self, s, d):
        raw = json.dumps(d).encode(); self.send_response(s)
        self.send_header("Content-Type","application/json"); self.send_header("Content-Length",str(len(raw)))
        self.end_headers(); self.wfile.write(raw)
    def _body(self):
        n = int(self.headers.get("Content-Length","0") or "0")
        return json.loads(self.rfile.read(n).decode()) if n else {}
    def do_GET(self):
        self._j(200, {"ok": True}) if self.path == "/health" else self._j(404, {"error":"not found"})
    def do_POST(self):
        if TOKEN and self.headers.get("Authorization") != f"Bearer {TOKEN}":
            self._j(401, {"error":"unauthorized"}); return
        p = self._body()
        if self.path == "/action":
            host = PTPHostClient.from_payload(p)
            if host: host.log("info", "myplugin.action", p.get("action"))
            self._j(200, {"ok": True, "output": {"echo": (p.get("input") or {})}})
        else:
            self._j(404, {"error":"not found"})
    def log_message(self, *a): pass

if __name__ == "__main__":
    ThreadingHTTPServer(("127.0.0.1", int(os.environ.get("PTP_PLUGIN_PORT","19090"))), H).serve_forever()
```

## plugin.json (the manifest)

```json
{
  "id": "my-plugin",
  "name": "My Plugin",
  "version": "0.1.0",
  "api_version": "1",
  "app_version": ">=0.1.0",
  "runtime": "external-http",
  "entry": { "command": "python", "args": ["plugin.py"], "health": "/health" },
  "permissions": ["log:write", "kv:read", "kv:write"],
  "actions": [{ "name": "ping", "label": "Ping" }],
  "config_schema": [{ "key": "prefix", "label": "Prefix", "type": "text", "default": "pong" }],
  "schedule": [{ "id": "tick", "label": "Tick", "action": "ping", "cron_config_key": "cron" }],
  "contributes": { "commands": [{ "id": "my-plugin.ping", "label": "Ping", "action": "ping" }] }
}
```

Validate against the schema: `ptp-pluginctl validate .`. The full JSON schema is
`plugin-manifest.schema.json` (shipped with the SDK / vendored via `--vendor-tools`).

### Optional declarations

- `resource_access` `{fs_paths, exec_binaries, net_hosts}` — declare what your plugin touches; shown
  to the admin at install. External-http plugins are **unrestricted host subprocesses** (full fs +
  subprocess + outbound network, same OS user) — there's no OS sandbox, so this is **declared, not
  enforced**. Be honest so admins know what they're installing.
- `requirements` `["clouddrive", "Pillow>=10"]` — pip lines; the host `pip install`s them into the
  plugin's `.deps` and puts it on `PYTHONPATH` at launch (best-effort).
- `contributes.views` — ship a `web/index.html` dashboard rendered in a sandboxed iframe.
- `category` (e.g. `"媒体库"`, `"短剧"`, `"工具"`) — a display-only tag used to group/filter plugins in
  the market table. It lives in **market metadata only**, never in the signed archive bytes, so adding
  or changing it does **not** require re-signing. Optional; plugins without it show `-` in the 分类 column.

## The request the host sends you

`POST /action` and `POST /event` both receive:

```json
{ "action": "ping", "input": {}, "config": { "prefix": "pong" }, "host": { "base_url": "http://127.0.0.1:PORT", "token": "..." } }
```

`config` is your merged config; `host` lets you build a `PTPHostClient.from_payload(...)`. Respond to
`/action` with `{ "ok": true, "output": {...} }` or `{ "ok": false, "error": "..." }`.

## Host API (you call these)

`PTPHostClient` (bearer-auth, env: `PTP_HOST_URL` / `PTP_HOST_TOKEN`):

| Method | Permission | Purpose |
|---|---|---|
| `log(level,event,msg,data)` | `log:write` | structured log |
| `notice(title,body,level)` | `notice:write` | push a notification |
| `publish(type,data)` | `event:publish` | emit an event |
| `kv_get/set/delete(key)` | `kv:read/write/delete` | persistent key-value |
| `config()` / `set_runtime_config(k,v)` | `config:read` / `config:write` | read/merge config |
| `sites()` / `site_cookie(id)` | `site:read` / `site:cookie` | configured PT sites + decrypted cookie |
| `media_search(keyword,year)` | `media:search` | reuse host TMDB/豆瓣/寸光集 metadata |
| `capabilities()` | — | host capabilities + your permissions |

Permissions are accepted at install (admin must approve the exact set) + wildcards (`*`, `host:*`,
`plugin:*`, `kv:*`). Host capabilities: `host.runtime.external-http.command/base-url`,
`host.event.subscription`, `host.schedule.cron`, `host.api.*` (config/log/notice/event/kv/site/media),
`host.ui.view/bridge` — inferred from your manifest (`entry.command`, `events`, `schedule`, `views`, `permissions`).

## Long-running work MUST be async

Each `/action` call is capped at ~30s by the host. Batch jobs (scanning a library, scraping many
items) **must return immediately** and run the work on a background thread, reporting progress via
`log`/KV and completion via `notice`. Expose a `status` action for polling. (Python `threading`;
Node `setImmediate`/workers.) For external HTTP, guard the **total** time with a deadline —
`urllib`/`fetch` socket timeouts only guard each recv, not slow-drip responses.

## Schedule (cron)

Declare `schedule[]` in the manifest; the host calls your action on cron. Use `cron_config_key` to let
the user control/empty-it via config (empty = disabled). You cannot self-schedule from outside the
manifest — but your process IS long-running, so background threads are fine.

## Test without the closed binary

- `ptp-pluginctl mock-host` — a fake Host API server for local dev.
- `ptp-pluginctl run <dir> --action <name> --once` — launches your plugin, health-checks, invokes an
  action/event against the mock host, prints the response.
- If you run PTPatronus yourself, `ptp-pluginctl dev <dir> --plugin-root data/plugins` mirrors your
  source into the live install and hot-reloads on changes.

## Package & publish

```bash
ptp-pluginctl keygen --private-key-out market.private --public-key-out market.public   # one-time market key
ptp-pluginctl package . --private-key market.private --market plugin-market.json \
    --archive-ref https://example.com/releases/my-plugin-0.1.0.zip --json-out dist/plugin-package.json
```

Publish either (a) to your **own** signed market (a public `plugin-market.json` + zip URLs), or (b)
submit your `plugin-package.json` to the **official market** repo. An official market repo can
aggregate many plugins: `ptp-pluginctl market aggregate --packages-dir packages --public-key
market.public --out plugin-market.json`. Admins install from a market URL in the app's plugin page;
trust levels are `verified-signed` / `signed` / `publisher-verified` / `unsigned`.

## Reference plugins

Real examples (in the PTPatronus official market): `ptp-ffmpeg-thumb`, `ptp-playlet-category`,
`ptp-drama-scrape` (寸光集 default source), `ptp-library-scrape` (uses `media_search`), `ptp-cd2-assistant`
(uses `requirements` + a dashboard view). They demonstrate the async-worker pattern, deadline-guarded
HTTP, NFO writing, and the install/market flow.

## Contract versioning

The plugin contract is `api_version: "1"`. Breaking changes bump the contract version and the dev kit
major version. The dev kit package version (e.g. `0.1.0`) is independent of the PTPatronus app version.
