# PTPatronus Plugin Development Kit

PTPatronus is **closed-source**; the plugin platform is an **open developer surface**. This directory
is the public dev kit (PDK) — everything a third-party developer needs to build, test, and publish
external-http plugins, with **zero host source**. Developers target the stable plugin contract
(`api_version: "1"`).

## What's here

| Path | What |
|---|---|
| [`DEVGUIDE.md`](DEVGUIDE.md) | The canonical public developer guide (read this first) |
| `python/` | `ptpatronus-plugin-sdk` — pip package: Python SDK (`PTPHostClient`) + `ptp-pluginctl` CLI |
| `typescript/` | `@ptpatronus/plugin-sdk` — npm package: TypeScript SDK (`PTPHostClient` + `PTPPluginViewClient`) |
| `install.sh` | curl bootstrap (no-pip fallback) — fetches `pluginctl.py` + schema |
| `publish.sh` | builds all artifacts + prints the upload commands |

## Install (3 ways)

```bash
# 1) Python SDK + CLI (recommended)
pip install ptpatronus-plugin-sdk        # -> ptp-pluginctl + from ptpatronus_plugin_sdk import PTPHostClient

# 2) TypeScript SDK (for Node/TS plugins + view bridges)
npm install @ptpatronus/plugin-sdk

# 3) No pip / no npm — just the tool, into the current dir
curl -fsSL https://ptang.top/devkit/install.sh | sh
```

## 30-second start

```bash
pip install ptpatronus-plugin-sdk
ptp-pluginctl create my-plugin --directory my-plugin
ptp-pluginctl mock-host &              # fake Host API — no closed binary needed
ptp-pluginctl run my-plugin --action ping --once
ptp-pluginctl package my-plugin --json-out dist/plugin-package.json
```

See [`DEVGUIDE.md`](DEVGUIDE.md) for the full contract (manifest schema, Host API, permissions,
async-worker pattern, packaging + market publishing).

## Single source of truth

`pluginctl.py` is canonical at `backend/tools/pluginctl.py` (used by the host's own vendoring/CI).
`sdk/python/sync.sh` copies it + the schema into the pip package at build time; a CI staleness check
(`sync.sh --check`) keeps them aligned. The dev kit version (`0.1.0`) is independent of the app;
the plugin contract stays `api_version: "1"`.

## Publishing (maintainers)

```bash
./publish.sh                            # builds sdist+wheel + npm tarball, prints upload commands
twine upload python/dist/*              # your PyPI creds
( cd typescript && npm publish )        # your npm creds (+ @ptpatronus org)
```
