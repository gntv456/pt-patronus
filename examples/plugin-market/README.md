# PTPatronus Plugin Market (reference sample)

This directory is a reference layout for an **official PTPatronus plugin market**
repository. It shows the directory structure, the per-plugin metadata that feeds the
index, and the CI workflow that regenerates `plugin-market.json` automatically.

A real market repo is generated with:

```bash
python backend/tools/pluginctl.py market init my-market \
  --name "PTPatronus Plugin Market" \
  --generate-keys \
  --release-base https://github.com/<org>/<market>/releases/download
```

`market init` also vendors `pluginctl.py` into `tools/ptpatronus/`; that vendored copy is
omitted here to avoid duplicating [`backend/tools/pluginctl.py`](../../backend/tools/pluginctl.py)
in this repo. The commands below use the canonical path instead.

## Layout

```text
plugin-market.json              # aggregated index (generated; commit this)
packages/                       # submitted plugin-package.json, one per plugin
keys/                           # market Ed25519 public key (committed) + private key (gitignored)
tools/ptpatronus/pluginctl.py   # vendored tool (added by `market init`; omitted in this sample)
.github/workflows/aggregate.yml # rebuilds the index on changes to packages/
```

The market repo holds **metadata only**. Each plugin's zip archive continues to live in
that plugin's own GitHub Releases; `packages/<plugin-id>.json` already carries the full
archive URL emitted by the plugin's CI.

## Sample contents

- `packages/external-http-demo.json` — the metadata produced by
  `package --json-out` for the [`external-http-demo`](../plugins/external-http-demo) plugin.
- `plugin-market.json` — the aggregated index built from that one entry.

## Rebuild the index

```bash
python backend/tools/pluginctl.py market aggregate \
  --packages-dir packages \
  --public-key keys/plugin-market.public.key \
  --out plugin-market.json
```

For a signed market, every entry in `packages/` must carry a `signature` produced with the
matching `PTP_PLUGIN_MARKET_PRIVATE_KEY` in the plugin's CI. See
[README/15-plugin-development.md → Official plugin market](../../README/15-plugin-development.md#official-plugin-market)
for the full flow.
