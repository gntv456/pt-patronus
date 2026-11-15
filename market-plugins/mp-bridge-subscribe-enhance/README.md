# mp-bridge-subscribe-enhance

订阅增强：热榜一键订阅 + 批量检查 + 订阅报告。**独立实现，功能对标 MoviePilot 订阅增强类插件**（未使用其任何代码）。

## 功能

- **批量检查**：逐个 force-check 全部订阅（搜索+推送下载器），有新增命中才通知（可配间隔秒减压）
- **热榜速览**：豆瓣/TMDB/Bangumi 热榜条目，标注哪些已订阅
- **从热榜订阅**：按条目一键建订阅（source+source_id 去重跳过）
- **订阅报告**：总数/类型分布/状态分布汇总，每周一通知

## 能力来源（宿主 Tool RPC）

| 工具 | 权限 |
|---|---|
| `subscribe_check` / `subscribe_create` | `subscribe:write` |
| `subscribe_list` / `recommend_feeds` | `subscribe:read` / `media:read` |
| `send_notification` / KV / 日志 | `notice:write` / `kv:read,write` / `log:write` |

## 配置

| 键 | 说明 | 默认 |
|---|---|---|
| `cron` | 批量检查定时 | `0 7 * * *` |
| `report_cron` | 周报定时 | `0 8 * * 1` |
| `check_notify` | 有新增命中才通知 | 开 |
| `report_notify` | 周报通知 | 开 |
| `check_concurrency_note` | 检查间隔秒 | 3 |

## 使用

启用即按 cron 自动批量检查 + 周报；「热榜速览」→「从热榜订阅」链路面板可用。
