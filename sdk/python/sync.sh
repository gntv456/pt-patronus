#!/usr/bin/env bash
# Sync the canonical dev-tool assets from the closed host repo into this public package.
# Canonical source of truth: backend/tools/pluginctl.py + backend/tools/plugin-manifest.schema.json.
# Run before `python -m build` / publishing.
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
SRC="$ROOT/backend/tools"
DST="$HERE/src/ptpatronus_plugin_sdk"

# CI staleness guard: compare WITHOUT copying; fail if the committed copy is stale.
if [ "${1:-}" = "--check" ]; then
    if ! diff -q "$SRC/pluginctl.py" "$DST/pluginctl.py" >/dev/null \
       || ! diff -q "$SRC/plugin-manifest.schema.json" "$DST/plugin-manifest.schema.json" >/dev/null; then
        echo "ERROR: sdk/python copy is stale vs backend/tools — run ./sync.sh" >&2
        exit 1
    fi
    echo "sdk/python in sync."
    exit 0
fi

cp "$SRC/pluginctl.py" "$DST/pluginctl.py"
cp "$SRC/plugin-manifest.schema.json" "$DST/plugin-manifest.schema.json"
echo "synced pluginctl.py + plugin-manifest.schema.json -> $DST"
