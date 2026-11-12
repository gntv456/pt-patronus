#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 The PTPatronus Authors
"""打包 extension/ 为 backend/internal/api/v1/extension_bundle.zip。

改动扩展源文件后重跑本脚本（CI 校验 zip 与源目录一致，过期即红）。
排除 test/ 目录与 *.test.js——运行时用不到。
"""
import os
import sys
import zipfile

ROOT = os.path.join(os.path.dirname(__file__), "..")
OUT = os.path.join(ROOT, "backend/internal/api/v1/extension_bundle.zip")


def main() -> int:
    src = os.path.join(ROOT, "extension")
    if not os.path.isdir(src):
        print("extension/ 不存在", file=sys.stderr)
        return 1
    files = []
    for dp, _, fns in os.walk(src):
        for fn in sorted(fns):
            p = os.path.join(dp, fn).replace("\\", "/")
            rel = os.path.relpath(p, src).replace("\\", "/")
            if rel.startswith("test/") or fn.endswith(".test.js"):
                continue
            files.append((p, rel))
    files.sort(key=lambda x: x[1])
    with zipfile.ZipFile(OUT, "w", zipfile.ZIP_DEFLATED) as z:
        for abs_p, rel in files:
            z.write(abs_p, rel)
    print(f"packed {len(files)} files -> {OUT}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
