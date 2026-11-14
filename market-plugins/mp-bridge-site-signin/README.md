# mp-bridge-site-signin

全站定时签到 + 签到结果通知。**独立实现，功能对标 MoviePilot 签到类插件**（未使用其任何代码）。

## 功能

- **每日签到**：cron 调度触发宿主 `site_checkin_all`，全站签到
- **结果通知**：成功/跳过/失败汇总；有失败自动升 warning 并列明失败站点
- **站内消息**：手动拉取全站收件箱（inbox/sent/system）
- **优惠速览**：读取全站当前 freeleech/促销
- **上次结果**：面板随时查看 KV 缓存的最近签到汇总

## 能力来源（宿主 Tool RPC）

| 工具 | 权限 |
|---|---|
| `site_checkin_all` | `site:action` |
| `site_messages` / `site_freeleech` | `site:read` |
| 通知 / 日志 / KV | `notice:write` / `log:write`（宿主内建） |

## 配置

| 键 | 说明 | 默认 |
|---|---|---|
| `cron` | 签到定时（5 段 cron，留空停用） | `0 9 * * *` |
| `notify` | 签到后发通知 | 开 |
| `notify_on_success` | 全部成功也通知 | 关 |
| `notify_level` | 通知级别（info/success/warning） | info |

## 使用

安装后启用即按 cron 自动签到；「插件面板 → 立即签到」可手动触发。
