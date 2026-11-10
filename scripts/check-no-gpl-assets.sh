#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 The PTPatronus Authors

# scripts/check-no-gpl-assets.sh — 断言构建产物里不含 GPL 资源。
#
# 为什么需要它：jessibuca（摄像头预览解码器）是 GPL-3.0，已被 .gitignore /
# .dockerignore 排除。但 vite 构建时会把 frontend/public/ **原样拷贝**进 dist——
# 若开发者本地跑过 fetch-jessibuca.sh，public/ 下就残留了这三个二进制，
# 下一次 npm run build 会把它们带进 dist，再被 go:embed 打进后端二进制，
# 最终随镜像发布出去。.dockerignore 拦不住这一步（构建发生在容器内、
# 用的是已经 COPY 进去的工作区内容），所以必须在 embed 之后断言。
#
# 用法：bash scripts/check-no-gpl-assets.sh [dist 目录]
set -euo pipefail

DIST="${1:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/backend/internal/web/dist}"

if [ ! -d "$DIST" ]; then
  echo "check-no-gpl-assets: 目录不存在 $DIST" >&2
  exit 1
fi

bad=0
for f in jessibuca.js decoder.js decoder.wasm; do
  hit="$(find "$DIST" -name "$f" -type f 2>/dev/null || true)"
  if [ -n "$hit" ]; then
    echo "✗ 构建产物含 GPL 资源：" >&2
    echo "$hit" | sed 's/^/    /' >&2
    bad=1
  fi
done

if [ "$bad" -ne 0 ]; then
  cat >&2 <<'EOF'

这些是 jessibuca（GPL-3.0）的播放器/解码器，不得进入对外分发的构建产物。
处理方式：
  1) 删掉 frontend/public/jessibuca/ 下的这三个文件（README.md 保留）
  2) 重新 npm run build
解码器会由后端在首次预览时自动获取到 DATA_DIR/jessibuca/，无需入库。
EOF
  exit 1
fi

echo "✓ 构建产物不含 GPL 资源（$DIST）"