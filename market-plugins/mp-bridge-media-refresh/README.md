# mp-bridge-media-refresh

媒体库巡检：定时全库扫描 + 元数据刷新 + 重复项报告。**独立实现，功能对标 MoviePilot 媒体库刷新类插件**（未使用其任何代码）。

## 功能

- **夜间全库扫描**：cron 触发 Emby/Plex/Jellyfin 全库扫描（排除库可配，扫描逐库细分）
- **元数据刷新**：指定 server+library 精准刷新
- **重复项报告**：逐服务器逐库检测重复媒体项，有重复才通知（周报 cron 可配）
- **失败通知**：扫描/检测失败自动升 warning

## 能力来源（宿主 Tool RPC）

| 工具 | 权限 |
|---|---|
| `mediaserver_library_scan` / `mediaserver_refresh` | `mediaserver:write` |
| `mediaserver_list` / `mediaserver_libraries` / `mediaserver_duplicates` | `mediaserver:read` |
| 通知 / KV / 日志 | `notice:write` / `kv:read,write` / `log:write` |

## 配置

| 键 | 说明 | 默认 |
|---|---|---|
| `cron` | 全库扫描定时 | `0 4 * * *` |
| `report_cron` | 重复项报告定时 | `30 4 * * 1` |
| `exclude_libraries` | 排除库名（逗号/换行） | 空 |
| `scan_notify` | 扫描后通知 | 关 |
| `dup_notify` | 有重复才通知 | 开 |

## 使用

启用即按 cron 夜间扫描 + 周一重复项报告；「立即全库扫描」面板命令可手动触发。
