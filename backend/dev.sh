#!/usr/bin/env bash
# 开发启动脚本（git bash / WSL）：加载 backend/.env → go run。
#
# 用法：
#   bash backend/dev.sh
#
# 首次使用（固化配置，之后任何终端/脚本都能拿到 env 帮你重启）：
#   cp backend/configs/config.example.yaml backend/.env
#   # 然后编辑 backend/.env，填入真实的 SECRET_KEY / ACTIVATION_SECRET 等
#
# 没有 .env 时回退用当前 shell 的环境变量（兼容你原来直接 go run 的方式）。
set -e
cd "$(dirname "$0")"          # 切到 backend/

if [ -f .env ]; then
  set -a; . ./.env; set +a
  echo "[dev] 已加载 backend/.env"
else
  echo "[dev] 未找到 backend/.env，用当前环境变量启动"
  echo "[dev] 建议固化配置：cp configs/config.example.yaml .env 并填真实值"
fi

echo "[dev] go run ./cmd/ptpatronus  (Ctrl+C 停止；改代码后 Ctrl+C 重跑即生效)"
exec go run ./cmd/ptpatronus
