# ptp-tgbot-bridge

Telegram Bot 桥接面。**独立实现，能力面打样模式**（issue #34 二梯队）：bot 轮询本体留在宿主进程（依赖进程内 AI 流式会话 `TalkFunc`），本插件承接配置后验证、健康巡检与 AI 单发对话。

## 功能

- **健康巡检**（默认每 30 分钟）：配置已启用但 bot 未运行 → 告警 + 自动 reload（可关）；SAFE_MODE 属预期静默；一切正常不打扰
- **运行状态**：`config_enabled` / `running` / `safe_mode` 三态速览
- **应用配置变更**：宿主 TG bot 设置改动后触发轮询重载（无需重启宿主）
- **问 AI**：`agent_chat` 单发对话，返回聚合回复（需 AI 已配置）

## 配置

TG bot 本体配置（token/允许用户/API URL）仍在宿主「TG Bot」页维护，插件只读状态不写配置。

| 键 | 说明 | 默认 |
|---|---|---|
| `cron` | 巡检定时 | `*/30 * * * *` |
| `auto_reload` | 发现停跑自动重载 | 开 |
| `notify` | 巡检异常才通知 | 开 |

## 能力来源（宿主 Tool RPC）

| 工具 | 权限 |
|---|---|
| `agent_chat`（本插件打样新增） | `agent:read`（新增能力位） |
| `tgbot_status` / `tgbot_reload`（新增） | `notify:read` / `notify:write` |
| 通知 / KV / 日志 | `notice:write` / `kv:read,write` / `log:write` |
