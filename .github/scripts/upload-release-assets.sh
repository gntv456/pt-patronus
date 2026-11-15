#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 The PTPatronus Authors

set -euo pipefail

version="${1:-}"
if [[ -z "$version" ]]; then
  echo "::error::version argument is required"
  exit 2
fi
shift

# 部署目标只来自环境（CI secrets 注入）——不设 fallback，避免仓库泄漏部署信息。
DEPLOY_HOST="${DEPLOY_HOST:-}"
DEPLOY_PORT="${DEPLOY_PORT:-}"
DEPLOY_USER="${DEPLOY_USER:-}"
DEPLOY_DIR="${DEPLOY_DIR:-}"

if [[ -z "$DEPLOY_HOST" || -z "$DEPLOY_USER" || -z "$DEPLOY_DIR" ]]; then
  echo "::error::DEPLOY_HOST / DEPLOY_USER / DEPLOY_DIR must be set (repo secrets); refusing to guess"
  exit 2
fi
DEPLOY_PORT="${DEPLOY_PORT:-22}"

if [[ -z "${DEPLOY_KEY:-}" ]]; then
  echo "::warning::DEPLOY_KEY is not set; skipping upload to private release server"
  exit 0
fi

files=()
for file in "$@"; do
  if [[ -f "$file" ]]; then
    files+=("$file")
  else
    echo "::warning::skip missing file: $file"
  fi
done

if [[ "${#files[@]}" -eq 0 ]]; then
  echo "::warning::no release files to upload"
  exit 0
fi

mkdir -p "$RUNNER_TEMP/ssh" "$HOME/.ssh"
chmod 700 "$HOME/.ssh"

key_file="$RUNNER_TEMP/ssh/deploy_key"
known_hosts="$RUNNER_TEMP/ssh/known_hosts"
printf '%s\n' "$DEPLOY_KEY" | tr -d '\r' > "$key_file"
chmod 600 "$key_file"
touch "$known_hosts"

ssh-keyscan -p "$DEPLOY_PORT" -H "$DEPLOY_HOST" >> "$known_hosts" 2>/dev/null || true

# 双斜杠前缀防 MSYS/Git-Bash 路径改写：单斜杠 /root/... 会被 scp 当相对路径
# 改写成 C:/Program Files/Git/root/...（2026-11-10 1.03.28 上传事故实测）。
remote_dir="//${DEPLOY_DIR%/}/${version}"
ssh_opts=(
  -i "$key_file"
  -p "$DEPLOY_PORT"
  -o IdentitiesOnly=yes
  -o StrictHostKeyChecking=accept-new
  -o UserKnownHostsFile="$known_hosts"
)
scp_opts=(
  -i "$key_file"
  -P "$DEPLOY_PORT"
  -o IdentitiesOnly=yes
  -o StrictHostKeyChecking=accept-new
  -o UserKnownHostsFile="$known_hosts"
)

echo "Uploading ${#files[@]} file(s) to ${DEPLOY_USER}@${DEPLOY_HOST}:${remote_dir}/"
ssh "${ssh_opts[@]}" "${DEPLOY_USER}@${DEPLOY_HOST}" "mkdir -p '${remote_dir}'"
scp "${scp_opts[@]}" "${files[@]}" "${DEPLOY_USER}@${DEPLOY_HOST}:${remote_dir}/"

# —— 产物托管镜像（更新方式改造）——
# 把客户端产物按规范名 <platform>-<version>.<ext> 镜像到 <DEPLOY_DIR>/downloads/：
# 服务器启动+每小时扫描该目录自动算 sha256 挂进自托管清单（backend/internal/update/
# artifacts.go），各端 OTA/外链自动激活——「人工回填网盘直链」退役。
# 同 platform 多个文件（如 split apk）只取体积最大的一个，避免规范名互相覆盖。
map_platform() {
  local f; f="$(basename "${1,,}")"
  case "$f" in
    *.apk) echo "android" ;;
    *.hap) echo "ohos" ;;
    *.ipa|*ios*) echo "ios" ;;
    *macos*) echo "macos" ;;
    *windows*) echo "windows" ;;
    server-linux-amd64*) echo "server-linux-amd64" ;;
    server-linux-arm64*) echo "server-linux-arm64" ;;
    agent-linux-amd64*) echo "agent-linux-amd64" ;;
    agent-linux-arm64*) echo "agent-linux-arm64" ;;
    *linux-x64*|*linux*) echo "linux" ;;
    *web-dist*) echo "web" ;;
    *) echo "" ;;
  esac
}

ext_of() {
  local f; f="$(basename "${1,,}")"
  case "$f" in
    *.tar.gz) echo "tar.gz" ;;
    *.apk) echo "apk" ;;
    *.hap) echo "hap" ;;
    *.ipa) echo "ipa" ;;
    *.zip) echo "zip" ;;
    *.dmg) echo "dmg" ;;
    *.exe) echo "exe" ;;
    *.gz) echo "gz" ;;
    *) echo "" ;;
  esac
}

stage="$RUNNER_TEMP/managed-artifacts"
rm -rf "$stage"; mkdir -p "$stage"
# ls -S：按体积降序，同 platform 只收第一个（最大）文件。
for file in $(ls -S "${files[@]}" 2>/dev/null); do
  [[ -f "$file" ]] || continue
  platform="$(map_platform "$file")"
  [[ -n "$platform" ]] || continue
  ext="$(ext_of "$file")"
  [[ -n "$ext" ]] || continue
  staged="$stage/${platform}-${version}.${ext}"
  if [[ ! -f "$staged" ]]; then
    cp "$file" "$staged"
    echo "managed artifact: $(basename "$staged")  ←  $(basename "$file")"
  fi
done
if compgen -G "$stage/*" > /dev/null; then
  ssh "${ssh_opts[@]}" "${DEPLOY_USER}@${DEPLOY_HOST}" "mkdir -p '//${DEPLOY_DIR%/}/downloads'"
  scp "${scp_opts[@]}" "$stage"/* "${DEPLOY_USER}@${DEPLOY_HOST}:///${DEPLOY_DIR%/}/downloads/"
  echo "managed artifacts mirrored to ${DEPLOY_DIR}/downloads/"
else
  echo "::warning::no client artifacts matched managed naming; downloads/ not updated"
fi
