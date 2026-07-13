# Third-party Plugin Development

PTPatronus can stay closed source while exposing a broad external plugin platform. Third-party plugins run as isolated processes and communicate with the host over HTTP.

## Quick start

Create a plugin template:

```bash
python backend/tools/pluginctl.py create my-demo-plugin --directory data/plugins/my-demo-plugin
```

Create a TypeScript plugin with a packaged web view:

```bash
python backend/tools/pluginctl.py create my-ts-plugin --directory data/plugins/my-ts-plugin --template typescript --with-view
```

Create a self-contained plugin repo that vendors the official tooling and CI workflows:

```bash
python backend/tools/pluginctl.py create my-plugin-repo --directory my-plugin-repo --template typescript --with-view --vendor-tools --with-ci both
```

Then install the local toolchain once and build the runtime bundle:

```bash
cd data/plugins/my-ts-plugin
npm install
npm run build
```

For that scaffold:

- `src/` builds the plugin server into `dist/`
- `web-src/` builds the plugin view into `web/`
- `npm run build:view` rebuilds only the frontend view assets
- `npm run build` rebuilds both halves

Start PTPatronus, open the plugin page, then reload and enable the new plugin.

For a tight local loop, run the source watcher:

```bash
python backend/tools/pluginctl.py dev my-plugin-repo --plugin-root data/plugins
```

This validates the source tree, mirrors it into `data/plugins/{plugin_id}`, and keeps syncing on change. If the plugin root contains a `package.json` with a `build` script, `pluginctl.py dev` will run that build automatically before each sync.

For compiled plugins such as TypeScript, run a build step before each sync:

```bash
python backend/tools/pluginctl.py dev my-ts-plugin --plugin-root data/plugins --build-command "npm run build"
```

Use `--skip-build` if you want to disable the automatic `package.json` build detection for a specific run.

For an existing standalone plugin repository, you can vendor the official tool bundle and write workflow templates later:

```bash
python backend/tools/pluginctl.py vendor-tools .
python backend/tools/pluginctl.py ci . --provider both --vendor-tools
```

The generated workflow always validates and packages the plugin. On GitHub tag builds it also uploads the zip, `dist/plugin-package.json`, and any generated `dist/plugin-market.json` to a GitHub Release.

Useful CI variables and secrets:

- `PTP_APP_VERSION`: optional host version used when validating `app_version` constraints in a standalone plugin repo
- `PTP_PLUGIN_MARKET_PRIVATE_KEY`: optional base64 Ed25519 seed used to sign archives
- `PTP_PLUGIN_MARKET_NAME`: optional market display name
- `PTP_PLUGIN_MARKET_ARCHIVE_REF`: optional archive URL/path written into `plugin-market.json`; GitHub workflows auto-fill a release URL on tag builds if this is blank
- `PTP_PLUGIN_PUBLISHER_ID`, `PTP_PLUGIN_PUBLISHER_NAME`, `PTP_PLUGIN_PUBLISHER_WEBSITE`, `PTP_PLUGIN_PUBLISHER_VERIFIED`
- `PTP_PLUGIN_PUBLISHED_AT`: optional explicit release timestamp
- `PTP_PLUGIN_CHANGELOG_FILE`: optional text file path whose non-empty lines become repeated `--changelog` entries

Validate a plugin:

```bash
python backend/tools/pluginctl.py validate data/plugins/my-demo-plugin
```

Run a mock Host API without starting the full application:

```bash
python backend/tools/pluginctl.py mock-host --port 19091 --token dev-token
```

Run a plugin in a standalone local sandbox that mimics the host runtime:

```bash
python backend/tools/pluginctl.py run examples/plugins/external-http-demo --action ping --input '{"message":"hello"}' --once
```

This starts a mock Host API, injects the same runtime environment variables as the application, waits for `/health`, and can immediately publish an event or invoke an action.

For shell-friendly input on Windows, object arguments also accept `key=value` pairs:

```bash
python backend/tools/pluginctl.py run examples/plugins/external-http-demo --action ping --input message=hello --once
```

Package a plugin:

```bash
python backend/tools/pluginctl.py package data/plugins/my-demo-plugin --out-dir dist/plugins
```

The package command prints a market-ready JSON fragment with the archive path, SHA-256 hash, manifest metadata, and inferred compatibility data such as `host_capabilities`. If the plugin directory has a `package.json` `build` script, `package` will run it automatically before archiving unless `--skip-build` is provided.

Use `--json-out dist/plugin-package.json` when CI or release automation needs the same metadata as a file.

Generate a market signing keypair:

```bash
python backend/tools/pluginctl.py keygen --private-key-out .keys/plugin-market.private.key --public-key-out .keys/plugin-market.public.key
```

Sign any existing archive:

```bash
python backend/tools/pluginctl.py sign dist/plugins/my-demo-plugin-0.1.0.zip --private-key .keys/plugin-market.private.key
```

Package, sign, and upsert a market index in one step:

```bash
python backend/tools/pluginctl.py package data/plugins/my-demo-plugin --out-dir dist/plugins --private-key .keys/plugin-market.private.key --market dist/plugin-market.json --market-name "Acme Plugin Market" --publisher-id acme --publisher-name "Acme Plugins" --publisher-website https://example.com --publisher-verified --changelog "Initial release"
```

`keygen` writes raw base64 Ed25519 key material. `package --market` keeps the archive reference relative to the market JSON unless `--archive-ref` is provided explicitly.

Dependency semantics:

- Dependencies are required by default.
- Add `"optional": true` to surface a dependency in UI without blocking install or enable.
- Dependency version constraints use the same loose syntax as `app_version`.
- Disabling a plugin is blocked while enabled plugins still require it.
- Uninstalling a plugin is blocked while installed plugins still require it.

## Development hot reload

In debug mode, or when `PLUGIN_DEV_WATCH=1` is set, the host watches `data/plugins/*` and will:

- auto-load new external plugin directories
- auto-reload a plugin after source changes
- auto-unload a plugin after its source directory is removed

The poll interval defaults to 1000 ms and can be changed with:

```text
PLUGIN_DEV_WATCH_INTERVAL_MS=500
```

If your plugin source lives outside the application data directory, use `pluginctl.py dev` to mirror it into the watched plugin root. `--build-command` lets the same watcher support TypeScript or any other compiled plugin workflow.

For isolated debugging without starting PTPatronus, use `pluginctl.py run`.

## Runtime model

External plugins declare:

```json
{
  "runtime": "external-http",
  "api_version": "1",
  "app_version": ">=0.1.0",
  "host_capabilities": ["host.runtime.external-http.command", "host.ui.bridge"],
  "optional_host_capabilities": ["host.api.notice.write"],
  "dependencies": [
    { "id": "json-toolbox", "version": ">=1.0.0", "reason": "Shared JSON transforms" }
  ],
  "entry": {
    "command": "python",
    "args": ["plugin.py"],
    "health": "/health",
    "shutdown": "/shutdown"
  }
}
```

Compatibility rules:

- `api_version` must exactly match the host plugin API version.
- `app_version` may be blank or use comma-separated constraints such as `>=0.81.0, <1.0.0`.
- `host_capabilities` is optional and may be used to require specific host surfaces such as plugin views, bridge APIs, scheduling, or managed process launch.
- `optional_host_capabilities` lets a plugin advertise best-effort integrations that should not block install or enable.
- `dependencies` declares required or optional plugin prerequisites. Required dependencies must be installed, enabled, and version-matched before this plugin can be enabled.
- Supported operators are `>`, `>=`, `<`, `<=`, `=` and `==`.
- The host auto-infers some capability requirements from the manifest itself, for example `permissions`, `contributes.views`, `schedule`, and `entry.command` / `entry.base_url`.
- The backend, market installer, and `pluginctl.py validate` all enforce the same compatibility checks.
- Vendored standalone `pluginctl.py` copies can still validate manifests outside the main PTPatronus repo. Set `PTP_APP_VERSION` in CI if you want `app_version` constraints checked against a specific host release.

The host starts the process and injects:

- `PTP_PLUGIN_ID`
- `PTP_PLUGIN_PORT`
- `PTP_PLUGIN_TOKEN`
- `PTP_PLUGIN_BASE_URL`
- `PTP_PLUGIN_DIR`
- `PTP_HOST_URL`
- `PTP_HOST_TOKEN`
- `PTP_HOST_PROTOCOL`
- `PTP_HOST_PERMISSIONS`
- `PTP_HOST_CAPABILITIES`

The plugin implements:

- `GET /health`
- `POST /event`
- `POST /action`

The machine-readable manifest schema is in:

```text
backend/tools/plugin-manifest.schema.json
```

The host calls `/event` and `/action` with this shape:

```json
{
  "action": "ping",
  "input": {},
  "config": {},
  "host": {
    "protocol_version": 1,
    "plugin_id": "my-demo-plugin",
    "base_url": "http://127.0.0.1:12345",
    "token": "temporary-token",
    "permissions": ["log:write", "kv:read", "kv:write"],
    "host_capabilities": ["host.api.log.write", "host.api.kv.read", "host.api.kv.write"]
  }
}
```

Command-launched plugins can use the host environment variables. `base_url` plugins can use the `host` field in the request payload.

## Host API

All Host API requests use:

```http
Authorization: Bearer <PTP_HOST_TOKEN>
```

Available endpoints:

- `GET /capabilities`
- `GET /config`
- `POST /runtime/config`
- `POST /log`
- `POST /notice`
- `POST /events`
- `GET /kv/{key}`
- `PUT /kv/{key}`
- `DELETE /kv/{key}`
- `GET /sites`
- `GET /sites/{id}/cookie`

Permissions:

- `config:read`
- `config:write`
- `log:write`
- `notice:write`
- `event:publish`
- `kv:read`
- `kv:write`
- `kv:delete`
- `site:read`
- `site:cookie`
- `media:search` — call the host media chain (`POST /media/search {keyword, year}` → `{results:[{source,source_id,title,year,type,overview,poster_url,backdrop_url,rating,genres}]}`). Scrapers reuse the host's TMDB/豆瓣/寸光集 sources instead of shipping their own TMDB key.

Wildcards are supported for trusted plugins:

- `host:*`
- `plugin:*`
- `*`

Common host capability names:

- `host.runtime.external-http.command`
- `host.runtime.external-http.base-url`
- `host.event.subscription`
- `host.schedule.cron`
- `host.api.config.read`
- `host.api.config.write`
- `host.api.log.write`
- `host.api.notice.write`
- `host.api.event.publish`
- `host.api.kv.read`
- `host.api.kv.write`
- `host.api.kv.delete`
- `host.api.site.read`
- `host.api.site.cookie`
- `host.ui.view`
- `host.ui.bridge`

## Manifest declarations & runtime patterns

### Resource-access declarations (`resource_access`)
External-http plugins run as **unrestricted host subprocesses** (same OS user, full filesystem + subprocess + outbound network) — there is no OS-level sandbox. To keep administrators informed, declare the resources a plugin touches in `resource_access`; these are shown on the install-confirmation page (declared, not enforced):

```json
{
  "resource_access": {
    "fs_paths": ["/data/media"],
    "exec_binaries": ["ffmpeg", "ffprobe"],
    "net_hosts": ["ptang.top", "api.themoviedb.org"]
  }
}
```

### Python dependencies (`requirements`)
A plugin can declare pip requirements; on install the host runs `pip install --target {plugin_dir}/.deps` and prepends `.deps` to `PYTHONPATH` when launching the plugin. Best-effort: if `pip` is absent or install fails, install still succeeds (the plugin may run on stdlib alone).

```json
{ "requirements": ["clouddrive", "Pillow>=10"] }
```

### Long-running work must be async
The host caps each `/action` call at ~30s. Batch jobs (scanning a whole library, scraping many items) **must** return immediately and run the work on a background thread, reporting progress via KV/log and completion via notice. Expose a `status` action for polling. Pattern:

```python
import threading
_worker_lock = threading.Lock()
_worker_thread = None
_worker_status = {"running": False, "done": False}

def _worker(cfg):
    try:
        result = do_the_work(cfg, _worker_status)  # updates _worker_status as it goes
        _worker_status.update({"running": False, "done": True})
        notice("done", str(result))
    except Exception as e:
        _worker_status.update({"running": False, "done": True, "error": str(e)})

def start_scan(cfg):
    global _worker_thread
    with _worker_lock:
        if _worker_thread and _worker_thread.is_alive():
            return {"started": False, "already_running": True}
        _worker_thread = threading.Thread(target=_worker, args=(cfg,), daemon=True)
        _worker_thread.start()
        return {"started": True}
```

For external HTTP, `urllib.request.urlopen(timeout=)` only guards each recv — guard the **total** time with a thread + `join(deadline)` so a throttled/slow-drip response can't wedge the worker.

### Plugin-view → action bridge
Plugin views (iframes) can call a declared action and render its output via the injected bridge — enables dashboards that pull live data from the plugin:

```js
const res = await window.PTPatronus.action('info', { name: 'instance1' })
// res.output is whatever the action returns
```

## TypeScript SDK

The TypeScript SDK lives in `sdk/typescript`.

```ts
import { PTPHostClient, type ActionRequest } from '@ptpatronus/plugin-sdk'

export async function handleAction(payload: ActionRequest) {
  const host = PTPHostClient.fromPayload(payload) ?? PTPHostClient.fromEnv()
  if (!host?.hasCapability('host.api.kv.write')) throw new Error('Host KV write support is required')
  await host?.log('info', 'my-plugin.action', 'started')
  const current = await host?.kvGet<number>('count')
  const next = (current?.value ?? 0) + 1
  await host?.kvSet('count', next)
  return { ok: true, output: { next } }
}
```

## Distribution

External plugins are installed under:

```text
data/plugins/{plugin_id}
```

A market index points to zip archives:

```json
{
  "version": 1,
  "public_key": "base64-encoded-ed25519-public-key",
  "api_version": "1",
  "app_version": "0.81.01",
  "host_capabilities": ["host.runtime.external-http.command", "host.ui.view", "host.ui.bridge"],
  "plugins": [
    {
      "id": "my-demo-plugin",
      "name": "My Demo Plugin",
      "version": "0.1.0",
      "host_capabilities": ["host.runtime.external-http.command", "host.ui.view", "host.ui.bridge"],
      "optional_host_capabilities": ["host.api.notice.write"],
      "dependencies": [
        { "id": "json-toolbox", "version": ">=1.0.0" }
      ],
      "publisher": {
        "id": "acme",
        "name": "Acme Plugins",
        "website": "https://example.com",
        "verified": true
      },
      "published_at": "2026-07-09T00:00:00Z",
      "changelog": ["Added dashboard view", "Improved retry logic"],
      "runtime": "external-http",
      "archive": "archives/my-demo-plugin-0.1.0.zip",
      "sha256": "...",
      "signature": "base64-encoded-ed25519-signature"
    }
  ]
}
```

Unsigned markets are valid for local development. Once `public_key` is set, every plugin entry in that market must also carry a matching `signature`.

Market trust is surfaced to administrators as:

- `verified-signed`
- `signed`
- `publisher-verified`
- `unsigned`

Install requests sent through the application API must include the permission set the administrator accepted:

```json
{
  "source": "data/plugin-market.json",
  "id": "my-demo-plugin",
  "accepted_permissions": ["log:write", "kv:read", "kv:write"],
  "accepted_permissions_by_plugin": [
    { "id": "json-toolbox", "accepted_permissions": ["kv:read"] },
    { "id": "my-demo-plugin", "accepted_permissions": ["log:write", "kv:read", "kv:write"] }
  ]
}
```

The backend rejects the install if any accepted permission set does not match the market install plan.

## Upgrade and rollback

Market installs now keep local release snapshots under:

```text
data/../plugin-releases/{plugin_id}/
```

Each successful install or upgrade records a local release snapshot. Rolling back restores the selected snapshot without re-downloading the plugin package.

## Official plugin market

A single plugin's CI already emits a signed `plugin-package.json` (one `MarketPlugin` entry) via `package --json-out`. An **official market** holds many plugins, so `pluginctl.py market` aggregates those per-plugin files into one `plugin-market.json` and scaffolds the market repository.

### Aggregate multiple plugins

```bash
python backend/tools/pluginctl.py market aggregate \
  --packages-dir packages \
  --public-key keys/plugin-market.public.key \
  --out plugin-market.json \
  --name "PTPatronus Plugin Market"
```

- `--package <path>` (repeatable) and/or `--packages-dir <dir>` select the input `plugin-package.json` files. A directory scan keeps any JSON object that has `id`, `version`, and `archive`.
- `--keep latest` (default) keeps only the highest version per plugin id (with a warning listing the dropped versions); `--keep all` keeps every version.
- `--public-key` (base64 or a path) produces a **signed** market. Every input entry must then carry a `signature` (the all-or-nothing rule the host verifies with `verifySignature`); an unsigned entry aborts the build. Omit it for a local unsigned market, in which case entries must not carry signatures.
- `--archives-dir <dir>` is an optional integrity gate: for each archive resolvable in that directory the aggregator checks `sha256` and (for signed markets) the signature, aborting on mismatch. Archives not present locally are skipped with a warning — the host re-verifies bytes at install time regardless, so this is a publish-time safety net, not a requirement.
- `--private-key <key>` (+ `--archives-dir`) switches to **central re-sign**: the operator's market key re-hashes and re-signs every archive on disk, overwriting whatever signature each plugin arrived with. This is the curated-market model — authors submit unsigned zips and the market signing key never leaves the operator. It requires every archive to be present locally.
- `--release-base <url>` optionally rewrites every entry's `archive` to `{release_base}/{id}-v{version}/{id}-{version}.zip` (`--archive-pattern` overrides the template with `{release_base}`, `{id}`, `{version}`, `{name}`). Without it the aggregator keeps each entry's existing archive URL verbatim — which is what you want when each plugin's CI already filled a full release URL.

The output mirrors the index shape above: `version`, `name`, `api_version`, `app_version`, `host_capabilities`, `public_key` (signed markets only), and a `plugins` array sorted by id.

### Repository layout and flow

An official market repo holds **metadata only** — the zip archives continue to live in each plugin's own GitHub Releases:

```text
plugin-market.json              # aggregated index (generated; commit this)
packages/                       # submitted plugin-package.json, one per plugin
keys/plugin-market.public.key   # market Ed25519 public key (committed trust root)
keys/plugin-market.private.key  # matching seed (gitignored; CI secret only)
tools/ptpatronus/pluginctl.py   # vendored tool used to regenerate the index
.github/workflows/aggregate.yml # rebuilds the index on changes to packages/
```

The end-to-end flow:

1. Each plugin's CI runs `package ... --json-out dist/plugin-package.json` with `PTP_PLUGIN_MARKET_PRIVATE_KEY` set to the market signing key, so the metadata arrives already signed by the market key.
2. Drop the result into the market repo as `packages/<plugin-id>.json`.
3. `aggregate.yml` runs `market aggregate` on push and commits the regenerated `plugin-market.json`.

### Scaffold a new market repo

```bash
python backend/tools/pluginctl.py market init my-market \
  --name "My Plugin Market" \
  --generate-keys \
  --release-base https://github.com/<org>/<market>/releases/download
```

This writes the layout above (vendoring `pluginctl.py`, generating the keypair, and seeding an empty index and aggregate workflow). A reference sample lives under `examples/plugin-market/`. Keep `plugin-market.private.key` secret — it is gitignored and only ever provisioned to plugin CI as `PTP_PLUGIN_MARKET_PRIVATE_KEY`.

## UI contribution metadata

The manifest can already declare UI contribution intent, even while individual host UI surfaces are introduced over time:

```json
{
  "contributes": {
    "commands": [
      { "id": "my-plugin.ping", "label": "Ping", "action": "ping" }
    ],
    "menus": [
      { "location": "plugins/actions", "command": "my-plugin.ping" }
    ],
    "views": [
      { "id": "my-plugin.panel", "title": "My Panel", "location": "sidebar", "path": "/panel" }
    ]
  }
}
```

Plugin view assets are loaded from the plugin package `web/` directory through the authenticated plugin asset API. For example:

```text
data/plugins/my-plugin/web/index.html
```

Relative `script`, `link`, `img`, and similar asset URLs inside that HTML are resolved against the same plugin asset directory, so multi-file frontend bundles under `web/` work as expected.

and:

```json
{
  "contributes": {
    "views": [
      { "id": "dashboard", "title": "Dashboard", "location": "plugins", "path": "index.html" }
    ]
  }
}
```

The frontend renders plugin HTML in a sandboxed iframe using `srcdoc`. View-based plugins should declare or rely on inferred `host.ui.view` and `host.ui.bridge` capability requirements so markets can preflight them before installation.

## Plugin view bridge

Plugin views receive an injected `window.PTPatronus` object. The iframe never receives host tokens directly; requests are forwarded by the parent page to the protected backend bridge API.

```html
<script>
  async function run() {
    const current = await window.PTPatronus.kv.get('count')
    const next = Number(current.value || 0) + 1
    await window.PTPatronus.kv.set('count', next)
    await window.PTPatronus.log('info', 'ui.run', 'counter updated', { next })
    await window.PTPatronus.publish('plugin.demo.ui', { next })
  }
</script>
```

Bridge methods:

- `PTPatronus.capabilities()`
- `PTPatronus.config.get()`
- `PTPatronus.log(level, event, message, data)`
- `PTPatronus.notice(title, body, level?)`
- `PTPatronus.publish(type, data)`
- `PTPatronus.kv.get(key)`
- `PTPatronus.kv.set(key, value)`
- `PTPatronus.kv.delete(key)`
- `PTPatronus.sites.list()`

Bundled plugin frontends can also use the TypeScript SDK instead of the injected global directly:

```ts
import { PTPPluginViewClient } from '@ptpatronus/plugin-sdk'

const host = PTPPluginViewClient.fromWindow()
if (!host) throw new Error('plugin view bridge unavailable')

const caps = await host.capabilities()
if (host.hasPermission('kv:write')) {
  const current = await host.kvGet<number>('count')
  await host.kvSet('count', Number(current.value || 0) + 1)
}
```
