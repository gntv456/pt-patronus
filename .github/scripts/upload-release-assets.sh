#!/usr/bin/env bash
set -euo pipefail

version="${1:-}"
if [[ -z "$version" ]]; then
  echo "::error::version argument is required"
  exit 2
fi
shift

DEPLOY_HOST="${DEPLOY_HOST:-152.53.91.46}"
DEPLOY_PORT="${DEPLOY_PORT:-22}"
DEPLOY_USER="${DEPLOY_USER:-root}"
DEPLOY_DIR="${DEPLOY_DIR:-/root/ptpatronus-releases}"

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

remote_dir="${DEPLOY_DIR%/}/${version}"
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
