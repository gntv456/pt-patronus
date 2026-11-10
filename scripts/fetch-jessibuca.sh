#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 The PTPatronus Authors

# scripts/fetch-jessibuca.sh — 拉取摄像头预览所需的 jessibuca 播放器资源。
#
# 为什么不随仓库分发：jessibuca 是 GPL-3.0（见其 LICENSE）。把 decoder.wasm 提交进
# 公开仓库或打进对外发布的 Docker 镜像属于「分发」，会要求 PTPatronus 整体同样以
# GPL-3.0 发布。摄像头预览是可选功能，为它给整个产品套 GPL 不划算，故改为部署时
# 按需下载——仓库与镜像都不含该二进制，体积也不增加。
#
# 用法：
#   bash scripts/fetch-jessibuca.sh                 # 拉到 frontend/public/jessibuca/
#   bash scripts/fetch-jessibuca.sh /some/dir       # 拉到指定目录
#
# 幂等：已存在且 sha256 匹配则跳过。下载后逐个校验 sha256，不符即删除并报错退出——
# 上游 release 无签名，校验和是唯一的完整性依据。
set -euo pipefail

VERSION="v3.3.28"
BASE="https://raw.githubusercontent.com/langhuihui/jessibuca/${VERSION}/dist"
DEST="${1:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/frontend/public/jessibuca}"

# file:sha256 三元组。改动此处必须同步更新 frontend/src/utils/jessibucaAssets.ts。
ASSETS=(
  "jessibuca.js:be5880c00e253c1085c7332ca72a639ec526c07f2799a3d9f6dd30e9e4b6c9e9"
  "decoder.js:0f9c325d8a0a5c07d893d64007b3a4b9b29eb6fac752e7591fbe135b3051ed8c"
  "decoder.wasm:2f8638a5db551b5ab7df828522713f673e0104e2fe1be70b3d93857e487d7115"
)

mkdir -p "$DEST"
echo "jessibuca ${VERSION} -> ${DEST}"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

for entry in "${ASSETS[@]}"; do
  file="${entry%%:*}"
  want="${entry#*:}"

  # 幂等：已有且摘要匹配就跳过。
  if [ -f "$DEST/$file" ]; then
    have="$(sha256sum "$DEST/$file" | cut -d' ' -f1)"
    if [ "$have" = "$want" ]; then
      echo "  ✓ $file（已存在且校验通过）"
      continue
    fi
    echo "  ! $file 摘要不符，重新下载"
  fi

  echo "  ↓ $file"
  if ! curl -fsSL --retry 3 --retry-delay 1 -o "$tmp/$file" "$BASE/$file"; then
    echo "下载失败：$file（$BASE/$file）" >&2
    exit 1
  fi

  have="$(sha256sum "$tmp/$file" | cut -d' ' -f1)"
  if [ "$have" != "$want" ]; then
    echo "校验失败：$file" >&2
    echo "  期望 sha256 $want" >&2
    echo "  实际 sha256 $have" >&2
    exit 1
  fi
  mv "$tmp/$file" "$DEST/$file"
done

echo "完成。三个文件均已校验通过。"