#!/usr/bin/env bash
# 计算各端产物的 sha256，填入 update-manifest.json 的 targets.*.sha256
# 用法（发版后，产物就绪时）:
#   ./compute-sha256.sh dist/ptpatronus-agent-linux-amd64 \
#                      path/to/pt-patronus-0.70.01.apk \
#                      path/to/pt-patronus-windows-x64.zip
set -euo pipefail
if [ "$#" -eq 0 ]; then
  echo "用法: $0 <产物1> [产物2 ...]" >&2
  exit 1
fi
for f in "$@"; do
  if [ ! -f "$f" ]; then
    echo "跳过(不存在): $f" >&2
    continue
  fi
  printf '%s  %s\n' "$(sha256sum "$f" | awk '{print $1}')" "$f"
done
