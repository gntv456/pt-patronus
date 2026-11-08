#!/usr/bin/env bash
# 把 backend/definitions/*.yaml 同步到官方站点定义仓库 gntv456/ptpatronus-sites。
# 用法：bash scripts/sync-site-defs.sh ["同步说明"]
# 前置：gh auth 登录（write 权限）；主仓 definitions 是唯一编辑源——sites 仓库只读分发。
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MSG="${1:-sync definitions from ptpatronus $(date +%F)}"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

gh repo view gntv456/ptpatronus-sites --json name -q .name >/dev/null 2>&1 || {
  echo "✗ gntv456/ptpatronus-sites 不可访问（先 gh auth / gh repo create）"; exit 1; }

gh repo clone gntv456/ptpatronus-sites "$WORK/sites" -- --depth 1
cp "$ROOT"/backend/definitions/*.yaml "$WORK/sites/"
# 裸环境无 git 身份时内嵌（sites 仓提交统一走发布身份）
export GIT_AUTHOR_NAME="pt-patronus" GIT_AUTHOR_EMAIL="pt-patronus@users.noreply.github.com"
export GIT_COMMITTER_NAME="pt-patronus" GIT_COMMITTER_EMAIL="pt-patronus@users.noreply.github.com"
cd "$WORK/sites"
if git status --porcelain | grep -q .; then
  git add -A
  git commit -m "$MSG"
  git push origin main
  echo "✓ 已推送站点定义到 gntv456/ptpatronus-sites（实例将在下个 cron 周期/手动同步拿到）"
else
  echo "✓ 定义无变化"
fi
