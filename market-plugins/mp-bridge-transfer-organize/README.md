# mp-bridge-transfer-organize

下载完成自动整理（识别→重命名→硬链→刮削→入库）。**独立实现，功能对标 MoviePilot 自动整理类插件**（未使用其任何代码）。

## 功能

- **定时扫描**：cron 触发宿主 `transfer_sweep`，对已完成种子做全量整理（后台异步）
- **候选速览**：跨下载器列出 `state∈completed/seeding/uploading` 的种子
- **手动整理**：按 `downloader_id + hash` 整理单个种子
- **整理历史**：查成功/失败记录；失败按 `history_id` 一键重做
- **通知**：手动整理给结果；cron 扫描默认静音（可开）

## 能力来源（宿主 Tool RPC）

| 工具 | 权限 |
|---|---|
| `transfer_sweep` / `transfer_organize` / `transfer_redo` | `transfer:write` |
| `transfer_history` | `transfer:read` |
| `downloaders.list` / `torrents_list` | `downloader:read` |
| 通知 / 日志 / KV | `notice:write` / `log:write`（宿主内建） |

> 前置：宿主「整理」功能总开关需开启（`transfer_sweep` 在关闭时报错并提示）。

## 配置

| 键 | 说明 | 默认 |
|---|---|---|
| `cron` | 扫描定时（5 段 cron，留空停用） | `*/30 * * * *` |
| `completed_states` | 完成态关键字（逗号分隔） | `completed,seeding,uploading` |
| `notify` | cron 扫描后发通知 | 关 |

## 使用

安装后启用即按 cron 自动扫描；「插件面板 → 立即扫描」手动触发；失败历史可在面板重做。
