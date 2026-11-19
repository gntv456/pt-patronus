#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 The PTPatronus Authors
"""校验 extension_bundle.zip 与 extension/ 源目录逐文件一致（sha256）。

起因：extension_bundle.zip 是 embed 进后端二进制的分发产物（/system/extension.zip，
「关于页下载扩展」的数据源）。pack-extension.py 是手动重跑的——1.03.21 之后
extension/ 改了 9 个提交（全站改版/调色盘/UX 全集）bundle 一直没重打，用户
从关于页下载到的仍是旧代码，而 CI 与 pre-push 都没有拦（pack 脚本注释里声称
的「CI 校验过期即红」实际不存在）。本脚本补上这道门禁：过期即 exit 1。

用法：python scripts/check-extension-bundle.py
接线：CI extension job + pre-push 钩子。修复方式：python scripts/pack-extension.py。
"""
import hashlib
import os
import sys
import zipfile

ROOT = os.path.join(os.path.dirname(__file__), "..")
BUNDLE = os.path.join(ROOT, "backend", "internal", "api", "v1", "extension_bundle.zip")
SRC = os.path.join(ROOT, "extension")


def sha(b: bytes) -> str:
    return hashlib.sha256(b).hexdigest()


def main() -> int:
    if not os.path.exists(BUNDLE):
        print("[FAIL] extension_bundle.zip missing -- run python scripts/pack-extension.py", file=sys.stderr)
        return 1
    # 期望清单：源目录排除 test/ 与 *.test.js（与 pack-extension.py 同口径）
    expected = {}
    for dp, _, fns in os.walk(SRC):
        for fn in sorted(fns):
            p = os.path.join(dp, fn)
            rel = os.path.relpath(p, SRC).replace("\\", "/")
            if rel.startswith("test/") or fn.endswith(".test.js"):
                continue
            with open(p, "rb") as f:
                expected[rel] = sha(f.read())
    z = zipfile.ZipFile(BUNDLE)
    names = set(z.namelist())
    problems = []
    for rel, want in sorted(expected.items()):
        if rel not in names:
            problems.append(f"缺失: {rel}")
        elif sha(z.read(rel)) != want:
            problems.append(f"过期: {rel}")
    for n in sorted(names - set(expected)):
        problems.append(f"zip 多出: {n}")
    if problems:
        print(f"[FAIL] extension_bundle.zip differs from extension/ source ({len(problems)} issues):", file=sys.stderr)
        for p in problems[:10]:
            print("  " + p, file=sys.stderr)
        if len(problems) > 10:
            print(f"  ... total {len(problems)} issues", file=sys.stderr)
        print("Fix: python scripts/pack-extension.py", file=sys.stderr)
        return 1
    print(f"[OK] extension_bundle.zip fresh ({len(expected)} files match source)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
