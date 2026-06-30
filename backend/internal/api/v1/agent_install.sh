#!/bin/bash
# PTPatronus 监控 Agent 一键安装脚本（Linux，systemd）。
# 用法（由 dashboard 的 Agent 管理页生成，curl | bash）：
#   curl -fsSL <DASHBOARD>/api/v1/monitor/agents/install.sh | \
#     AGENT_ID=1 AGENT_SECRET=xxx DASHBOARD_URL=ws://panel:8088 bash
#
# 环境变量：
#   AGENT_ID      （必填）dashboard 创建 agent 时分配的 ID
#   AGENT_SECRET  （必填）dashboard 创建 agent 时下发的 secret（仅显示一次）
#   DASHBOARD_URL （必填）dashboard 的 WS 地址，如 ws://panel:8088 或 wss://panel.com
#   AGENT_REPO    （可选）GitHub 仓库，默认 gntv456/pt-patronus（fork 时改）
#   AGENT_DOWNLOAD_BASE （可选，闭源分发用）网盘直链基址，如 https://网盘/path/agent；
#                        设了则从 ${AGENT_DOWNLOAD_BASE}/ptpatronus-agent-linux-<arch> 下载，不走 GitHub
set -e

AGENT_ID="${AGENT_ID:-}"
AGENT_SECRET="${AGENT_SECRET:-}"
DASHBOARD_URL="${DASHBOARD_URL:-}"
AGENT_REPO="${AGENT_REPO:-gntv456/pt-patronus}"
AGENT_DOWNLOAD_BASE="${AGENT_DOWNLOAD_BASE:-}"
INSTALL_DIR="/opt/ptpatronus-agent"
ENV_FILE="/etc/ptpatronus-agent.env"
SERVICE="ptpatronus-agent"

err() { echo "[install] 错误：$*" >&2; exit 1; }

[ -z "$AGENT_ID" ] && err "缺少 AGENT_ID"
[ -z "$AGENT_SECRET" ] && err "缺少 AGENT_SECRET"
[ -z "$DASHBOARD_URL" ] && err "缺少 DASHBOARD_URL"
[ "$(id -u)" -ne 0 ] && err "请用 root 运行（需写 systemd unit 到 /etc/systemd/system）"

# 架构映射（仅支持 amd64/arm64）
case "$(uname -m)" in
  x86_64|amd64)  GOARCH=amd64 ;;
  aarch64|arm64) GOARCH=arm64 ;;
  *) err "不支持的架构 $(uname -m)（仅支持 amd64/arm64）" ;;
esac

# 下载二进制：闭源分发优先用 AGENT_DOWNLOAD_BASE（网盘直链）；未设则走 GitHub Release + ghproxy 镜像兜底
mkdir -p "$INSTALL_DIR"
ASSET="ptpatronus-agent-linux-${GOARCH}"
if [ -n "$AGENT_DOWNLOAD_BASE" ]; then
  URLS=( "${AGENT_DOWNLOAD_BASE%/}/${ASSET}" )
else
  URLS=(
    "https://github.com/${AGENT_REPO}/releases/latest/download/${ASSET}"
    "https://ghfast.top/https://github.com/${AGENT_REPO}/releases/latest/download/${ASSET}"
    "https://ghproxy.com/https://github.com/${AGENT_REPO}/releases/latest/download/${ASSET}"
  )
fi
echo "[install] 下载 agent（linux-${GOARCH}）..."
ok=0
for u in "${URLS[@]}"; do
  if curl -fsSL "$u" -o "$INSTALL_DIR/agent"; then ok=1; break; fi
  echo "[install] 源失败，换下一个：$u"
done
[ "$ok" = "1" ] || err "下载失败，请检查网络或手动下载 ${ASSET} 到 ${INSTALL_DIR}/agent"
chmod +x "$INSTALL_DIR/agent"

# secret 写独立 env 文件（umask 077 即时 600，避免进 systemd unit 明文被 systemctl show 泄露）
OLD_UMASK=$(umask); umask 077
cat > "$ENV_FILE" <<EOF
DASHBOARD_URL=${DASHBOARD_URL}
AGENT_ID=${AGENT_ID}
AGENT_SECRET=${AGENT_SECRET}
REPORT_INTERVAL=5
EOF
umask "$OLD_UMASK"

# systemd unit（开机自启 + 崩溃自动重启）
cat > /etc/systemd/system/${SERVICE}.service <<EOF
[Unit]
Description=PTPatronus Monitor Agent
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=${INSTALL_DIR}/agent
EnvironmentFile=${ENV_FILE}
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable --now ${SERVICE}

cat <<EOF

[install] 完成！agent 已启动并设为开机自启。
  查看状态：systemctl status ${SERVICE}
  查看日志：journalctl -u ${SERVICE} -f
  停止/卸载：systemctl disable --now ${SERVICE} && rm -rf ${INSTALL_DIR} ${ENV_FILE} /etc/systemd/system/${SERVICE}.service
EOF
