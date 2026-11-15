# mp-bridge-cloud-strm

网盘 STRM 生成联动。**独立实现，功能对标 MoviePilot 网盘 STRM 类插件**（未使用其任何代码）。

## 功能

- **夜间生成**：cron 同步按全部启用规则生成 .strm（宿主返回跨规则汇总，超时由 host 工具超时兜底）
- **单规则生成**：按 rule_id 精准触发
- **巡检报告**：规则数/启用数/网盘数/上次生成结果，weekly 通知；上次有失败升 warning
- **记录查询**：最近生成记录（可按网盘过滤）
- **静音语义**：夜间生成默认静音，`failed>0` 才升 warning

## 能力来源（宿主 Tool RPC）

| 工具 | 权限 |
|---|---|
| `cloud_generate_strm` / `cloud_generate_all` | `cloud:write` |
| `cloud_list` / `cloud_rules` / `cloud_records` | `cloud:read` |
| 通知 / KV / 日志 | `notice:write` / `kv:read,write` / `log:write` |

## 配置

| 键 | 说明 | 默认 |
|---|---|---|
| `cron` | 生成定时 | `0 3 * * *` |
| `report_cron` | 巡检定时 | `0 5 * * 1` |
| `generate_notify` | 生成失败才通知 | 开 |
| `health_notify` | 巡检通知 | 开 |

## 使用

规则本体在宿主「网盘」页维护（`cloud_rule_*` 工具面），本插件负责定时生成 + 健康巡检 + 失败告警。
