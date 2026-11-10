#!/usr/bin/env sh
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 The PTPatronus Authors

# Bootstrap the PTPatronus plugin dev tool WITHOUT pip — fetches pluginctl.py + the manifest
# schema into the current dir (or $1). For environments without PyPI access.
#
#   curl -fsSL https://ptang.top/devkit/install.sh | sh
#   BASE=https://your-mirror/devkit sh install.sh [destdir]
set -eu

BASE="${BASE:-https://ptang.top/devkit}"
DEST="${1:-.}"
mkdir -p "$DEST"

for f in pluginctl.py plugin-manifest.schema.json; do
  echo "fetching $BASE/$f -> $DEST/$f"
  curl -fsSL "$BASE/$f" -o "$DEST/$f"
done

echo "done. run:  python $DEST/pluginctl.py --help"
echo "(or prefer: pip install ptpatronus-plugin-sdk  ->  ptp-pluginctl)"
