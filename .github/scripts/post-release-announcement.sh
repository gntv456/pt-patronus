#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 The PTPatronus Authors
# 发版后自动在 Discussions → Announcements 发公告帖（发版自动化最后一环）。
# 调用方：flutter-release.yml publish-release 的 "Post release announcement" 步骤。
#
# 依赖（仓库 Variables，非敏感，gh variable set 配置一次）：
#   DISCUSSION_REPO_ID / DISCUSSION_ANNOUNCE_CATEGORY_ID
# 输入：$1 = 版本号（无 v 前缀）；CHANGELOG.md 需有该版本段落（无段落则跳过不报错）。
# 已有同名标题的帖子时跳过（幂等：rerun 不重复发）。
set -euo pipefail

ver="${1:?用法: post-release-announcement.sh <版本号>}"
title="v${ver} 发布公告"

root="$(cd "$(dirname "$0")/../.." && pwd)"
body="$("$root/.github/scripts/extract-changelog.sh" "$ver")"
if [ -z "$body" ]; then
  echo "[announce] CHANGELOG 无 ${ver} 段落，跳过公告"
  exit 0
fi
body="${body}

---
下载与升级见 [Release v${ver}](https://github.com/gntv456/pt-patronus/releases/tag/v${ver})；Docker 用户 \`docker compose pull && docker compose up -d\`。升级务必保留 ./data 卷。"

repo_id="${DISCUSSION_REPO_ID:?缺 DISCUSSION_REPO_ID（gh variable set 配置）}"
cat_id="${DISCUSSION_ANNOUNCE_CATEGORY_ID:?缺 DISCUSSION_ANNOUNCE_CATEGORY_ID}"

# 幂等：查已有同名帖
existing="$(gh api graphql -f query='{ repository(owner: "gntv456", name: "pt-patronus") { discussions(first: 20, categoryId: "'"$cat_id"'") { nodes { title } } } }' --jq ".data.repository.discussions.nodes[] | select(.title == \"$title\") | .title" 2>/dev/null || true)"
if [ -n "$existing" ]; then
  echo "[announce] 已有同名公告帖，跳过"
  exit 0
fi

payload="$(mktemp)"
trap 'rm -f "$payload"' EXIT
cat > "$payload" <<VAR_EOF
{"query":"mutation(\$repo: ID!, \$cat: ID!, \$title: String!, \$body: String!) { createDiscussion(input: {repositoryId: \$repo, categoryId: \$cat, title: \$title, body: \$body}) { discussion { url } } }","variables":{"repo":"$repo_id","cat":"$cat_id","title":"$title","body":$(python -c 'import json,sys;print(json.dumps(sys.stdin.read()))' <<< "$body")}}
VAR_EOF
gh api graphql --input "$payload" --jq '.data.createDiscussion.discussion.url'
