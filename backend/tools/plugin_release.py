# -*- coding: utf-8 -*-
"""Bump patch versions and package market plugins for release.

Usage:
  python backend/tools/plugin_release.py <plugin_id> [plugin_id ...] \
      [--private-key KEY] [--out-dir dist/plugins]

Without --private-key the archive is packaged unsigned; the market index still
records sha256. Set PTP_PLUGIN_MARKET_PRIVATE_KEY (base64 seed or key path) in CI
to produce signed releases.
"""
from __future__ import annotations

import argparse
import json
import re
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
CTL = ROOT / "backend" / "tools" / "pluginctl.py"
PLUGINS = ROOT / "market-plugins"


def bump_patch(manifest: Path) -> str:
    text = manifest.read_text(encoding="utf-8")
    data = json.loads(text)
    old = str(data.get("version", "0.0.0"))
    match = re.match(r"^(\d+)\.(\d+)\.(\d+)(.*)$", old)
    if not match:
        raise SystemExit(f"unsupported version format: {old}")
    new = f"{match.group(1)}.{match.group(2)}.{int(match.group(3)) + 1}{match.group(4)}"
    needle = f'"version": "{old}"'
    if needle not in text:
        raise SystemExit(f"version literal not found in {manifest}")
    manifest.write_text(text.replace(needle, f'"version": "{new}"', 1), encoding="utf-8")
    return new


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("plugins", nargs="+")
    parser.add_argument("--private-key", default=None)
    parser.add_argument("--out-dir", default="dist/plugins")
    parser.add_argument("--market", default="dist/plugin-market.json")
    parser.add_argument("--market-name", default="PTPatronus 官方插件市场")
    args = parser.parse_args()

    out_dir = (ROOT / args.out_dir).resolve()
    market = (ROOT / args.market).resolve()
    out_dir.mkdir(parents=True, exist_ok=True)
    market.parent.mkdir(parents=True, exist_ok=True)

    key = args.private_key or __import__("os").environ.get("PTP_PLUGIN_MARKET_PRIVATE_KEY")
    for plugin_id in args.plugins:
        plugin_dir = PLUGINS / plugin_id
        manifest = plugin_dir / "plugin.json"
        if not manifest.exists():
            raise SystemExit(f"plugin not found: {plugin_id}")
        new_version = bump_patch(manifest)
        cmd = [sys.executable, str(CTL), "package", str(plugin_dir),
               "--out-dir", str(out_dir), "--market", str(market),
               "--market-name", args.market_name,
               "--json-out", str(out_dir / f"{plugin_id}-{new_version}.json")]
        if key:
            cmd += ["--private-key", key]
        print(f"[{plugin_id}] {new_version}: packaging ...", flush=True)
        subprocess.run(cmd, check=True, cwd=str(ROOT))
        print(f"[{plugin_id}] done", flush=True)
    print(f"market index: {market}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
