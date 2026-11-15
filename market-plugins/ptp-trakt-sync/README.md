# ptp-trakt-sync

Trakt.tv watchlist → 宿主订阅同步。**issue #34「核心功能出墙」的打样实现**：原 `internal/trakt` 进程内模块的替代品，全部经插件契约（external-http + Tool RPC），零宿主代码。

## 授权（device code，无需回调地址）

1. [trakt.tv/settings/applications](https://trakt.tv/settings/applications) 新建应用（回调留空即可），取 Client ID / Secret 填入插件设置
2. 「开始设备授权」→ 得到 `user_code` 与验证 URL
3. 浏览器打开 URL 输入 user_code → 「检查授权结果」完成换取
4. token 存插件 KV，到期自动用 refresh token 续期

## 同步

- 拉取 `/users/me/watchlist`（电影/剧集可分别开关）
- Trakt 条目 → **TMDB ID 映射**（ids.tmdb 直取，缺则 Trakt Search slug 回查）
- 按 `source=tmdb + source_id` 去重：已存在跳过
- 列表级 `last_updated_at` 锚点预留增量位；失败/无法映射的条目各自列明
- cron 每 6 小时增量同步；有新增才通知

## 能力来源（宿主 Tool RPC）

| 工具 | 权限 |
|---|---|
| `subscribe_create` | `subscribe:write` |
| `subscribe_list` | `subscribe:read` |
| 通知 / KV / 日志 | `notice:write` / `kv:read,write` / `log:write` |

Trakt API 由插件进程直连（公网第三方），不占用宿主代理面。

## 迁移说明

老用户若用过宿主内置 Trakt（已删除）：插件重新走一遍设备授权即可，watchlist 数据在 Trakt 侧无损。
