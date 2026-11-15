# ptp-cookiecloud-sync

CookieCloud 同步调度器。**独立实现，替代宿主进程内 cookiecloud-tick cron**（issue #34 二梯队；解密与站点匹配语义留在宿主链，本插件承接调度与通知）。

## 语义

- cron 触发宿主 `site_sync_cookies`（默认每 5 分钟，**沿用原软间隔语义**：是否真正同步仍由宿主按 `interval_min + last_sync` 决定）
- 可选回推（本地 cookie 合并写回云端）
- **有更新/新增才通知**（可配）；失败必通知（可关）

## 配置

CookieCloud 服务器地址 / UUID / 密码 / 同步间隔等仍在宿主「站点 → CookieCloud」页维护（`/api/v1/cookiecloud/config`），插件只读不写。

| 键 | 说明 | 默认 |
|---|---|---|
| `cron` | 同步定时 | `*/5 * * * *` |
| `push` | 同步时回推本地 cookie | 关 |
| `notify_updated` | 有更新/新增才通知 | 开 |
| `notify_errors` | 失败必通知 | 开 |

## 能力来源（宿主 Tool RPC）

| 工具 | 权限 |
|---|---|
| `site_sync_cookies`（本插件打样新增） | `site:action` |
| `site_sync_settings`（新增） | `site:read` |
| 通知 / KV / 日志 | `notice:write` / `kv:read,write` / `log:write` |
