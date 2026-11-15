# mp-bridge-torrent-editor

批量做种规则编辑器。**独立实现，功能对标 MoviePilot 批量改做种类插件**（未使用其任何代码）。

## 功能

- **圈选预览**：按状态/分类/标签（全含/任一）/名称关键字跨下载器圈选，只读不动数据
- **批量改分享限制**：`ratio_limit`（-2 无限 / -1 全局 / ≥0 数值）+ `seeding_time_limit`（分钟，同约定）
- **批量改标签**：整集替换标签集（set 语义）
- **规则模板**：条件+动作存 KV 命名模板；定时 cron 到点自动应用默认模板
- **安全语义**：`set_limits` 两个参数必填才执行（`ratio_limit=0` 会立即命中全部种子，工具层有防呆）

## 能力来源（宿主 Tool RPC）

| 工具 | 权限 |
|---|---|
| `torrents_set_share_limits` / `torrents_set_tags` | `downloader:write` |
| `downloaders.list` / `torrents_list` | `downloader:read` |
| 通知 / KV / 日志 | `notice:write` / `kv:read,write` / `log:write` |

## 配置

| 键 | 说明 | 默认 |
|---|---|---|
| `template_name` | 定时任务应用的模板名 | default |
| `notify` | 批量操作后通知 | 开 |
| `cron` | 定时应用模板 | 空（不启用） |

## 使用

先 `select` 预览条件命中 → 确认无误再 `set_limits` / `set_tags`；常用组合存模板挂 cron 自动应用。
