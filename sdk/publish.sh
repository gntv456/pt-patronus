#!/usr/bin/env sh
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 The PTPatronus Authors

# Build the public Plugin Development Kit artifacts. Does NOT upload (needs registry creds).
# Run on a machine with: Python 3.8+, `build` + `wheel` + `twine` (pip install build wheel twine),
# Node + npm, and TypeScript installed (npm install in sdk/typescript).
set -eu
HERE="$(cd "$(dirname "$0")" && pwd)"

echo "==> 1/4  sync canonical pluginctl + schema into sdk/python (single source of truth)"
"$HERE/python/sync.sh"

echo "==> 2/4  build python sdist + wheel"
(cd "$HERE/python" && python -m build)

echo "==> 3/4  build + pack typescript SDK"
(cd "$HERE/typescript" && npm install && npm run build && npm pack)

echo "==> 4/4  done. Artifacts:"
ls -1 "$HERE/python/dist" 2>/dev/null | sed 's/^/    python\/dist\//'
ls -1 "$HERE/typescript"/*.tgz 2>/dev/null | sed 's/^/    typescript\//'

cat <<EOF

Publish with YOUR registry credentials:

  # PyPI (Python SDK + ptp-pluginctl CLI)
  twine upload sdk/python/dist/*

  # npm (TypeScript SDK, scoped — needs the \@ptpatronus org)
  ( cd sdk/typescript && npm publish )

  # ptang.top devkit mirror (for the curl bootstrap):
  #   upload sdk/python/src/ptpatronus_plugin_sdk/pluginctl.py
  #         sdk/python/src/ptpatronus_plugin_sdk/plugin-manifest.schema.json
  #         sdk/install.sh  ->  https://ptang.top/devkit/{pluginctl.py,plugin-manifest.schema.json,install.sh}
EOF
