#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 The PTPatronus Authors
# 从 CHANGELOG.md 提取当前 tag 版本的段落，写入 GITHUB_OUTPUT 供 release step
# 用作 body。找不到对应段落时输出空串（release step 退回 generate_release_notes）。
#
# 用法（workflow 内）：
#   echo "body<<EOF" >> "$GITHUB_OUTPUT"; cat <<'BODY' >> ...
# 实际实现见 flutter-release.yml 的 "Extract changelog body" 步骤调用本脚本：
#   body="$(bash .github/scripts/extract-changelog.sh "${GITHUB_REF_NAME#v}")"
#   { echo "body<<CHANGELOG_EOF"; echo "$body"; echo "CHANGELOG_EOF"; } >> "$GITHUB_OUTPUT"
set -euo pipefail

ver="${1:?用法: extract-changelog.sh <版本号如 1.03.32>}"
root="$(cd "$(dirname "$0")/../.." && pwd)"
changelog="$root/CHANGELOG.md"

[ -f "$changelog" ] || { echo "[extract-changelog] CHANGELOG.md 不存在，跳过" >&2; exit 0; }

# 匹配 "## <ver> — <日期>" 标题，截取到下一个 "## " 标题或文件尾
awk -v ver="$ver" '
  $0 ~ "^## " ver "( |—|$)" { found=1; next }
  /^## / && found { exit }
  found { print }
' "$changelog" | sed '/./,$!d'   # 去头部空行

exit 0
