# API 参考

Base URL: `/api/v1`

## 认证

| 方式 | 头 | 说明 |
|---|---|---|
| JWT | `Authorization: Bearer <token>` | 登录获取 |
| API Key | `X-API-Key: <key>` | 程序化访问（env API_KEY 配置） |

## 公开端点（无需鉴权）

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/system/health` | 健康检查 |
| POST | `/auth/hxpt/captcha` | 好学账号登录 Step A：取登录页验证码/探测模式 → `{imagehash, image, need_captcha, challenge_mode, extra_form}` |
| POST | `/auth/hxpt/login` | 好学账号登录 Step B：`{username, password, imagehash?, imagestring?, two_step_code?, extra_form?}` → 验 App 专属 VIP → JWT |
| POST | `/auth/hxpt/cookie-login` | 好学 Cookie 登录（最稳，覆盖 2FA/挑战响应账号）：`{cookie}` → 验 VIP → JWT |
| GET | `/auth/status` | 登录/VIP 状态 |
| POST | `/auth/verify-vip` | 用 cookie 验好学 VIP |
| POST | `/auth/login` | （已停用）本地密码登录 → 410，请用好学账号/Cookie 登录 |
| POST | `/auth/setup` | （已停用）本地初始化 → 410 |
| POST | `/auth/activate` | （legacy）激活码，不再作准入 gate |

## 鉴权与会话

- 业务端点需 `Authorization: Bearer <jwt>` 且**当前用户为好学 App 专属 VIP**（按用户门禁，宽限期 `VIP_GRACE_DAYS` 内放行；超期 → 403 `vip_expired`）。
- `POST /auth/logout`（需鉴权）：bump `TokenVersion`，使当前用户所有已签发 JWT 立即失效。
- `X-API-Key` 旁路跳过 VIP 门禁，作程序化/应急 admin 入口。
- 登录响应：`{token, user:{id, username, hxpt_uid, hxpt_username, vip_level, is_admin}}`。首个好学 VIP 用户即管理员。

## 站点管理（需鉴权）

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/sites` | 列出全部站点 |
| POST | `/sites` | 创建站点（cookie 加密存储） |
| GET | `/sites/definitions` | 列出可用站点定义 |
| POST | `/sites/inspect` | 校准诊断（URL+cookie+选择器→命中结果+页面片段） |
| GET | `/sites/:id` | 取单个站点 |
| PUT | `/sites/:id` | 更新站点 |
| DELETE | `/sites/:id` | 删除站点 |
| POST | `/sites/:id/test` | 连接测试（登录态+用户等级） |
| POST | `/sites/:id/search` | 站内搜索（keyword） |
| POST | `/sites/:id/cookie` | 上传 cookie（WebView 登录后） |
| POST | `/sites/:id/checkin` | 单站签到（NexusPHP `attendance.php`；返回 `{success, message, bonus}`，不支持签到或未登录时 `success=false` 并带 error） |
| POST | `/sites/checkin` | 一键签到全部启用站点（并发，每站 30s 超时；不支持的站如馒头跳过不计失败，已签识别为「今天已签到」不重复签到） |
| GET | `/sites/checkin/records` | 签到历史（`?site_id=&limit=&offset=`，按时间倒序；含 cron 自动 + 手动触发，每条记 站点/结果/魔力/来源） |
| POST | `/search` | 跨站聚合搜索 |

## 下载器（需鉴权）

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/downloaders` | 列出下载器 |
| POST | `/downloaders` | 创建下载器 |
| PUT/DELETE | `/downloaders/:id` | 更新/删除 |
| POST | `/downloaders/:id/test` | 连接测试 |
| GET | `/downloaders/:id/torrents` | 种子列表 |
| POST | `/downloaders/:id/torrents` | 添加种子（magnet/url） |
| POST | `/downloaders/:id/pause` | 暂停 |
| POST | `/downloaders/:id/resume` | 恢复 |
| POST | `/downloaders/:id/remove` | 删除任务 |
| GET | `/downloaders/:id/stats` | 全局速率 |

## 仪表盘

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/dashboard` | 聚合统计（站点上传/下载/做种/魔力 + 下载器数） |
| POST | `/dashboard/sync` | 立即抓取所有启用站点统计并写快照，返回刷新后的总览（回填新加入站点的 0 数据，无需等 30 分钟定时同步） |
| GET | `/dashboard/trend?days=7` | 按天聚合趋势点 `{day, uploaded, downloaded, seeding, seeding_size, bonus, ratio}`（累积量先站×日 MAX 再跨站 SUM；ratio 由当日 uploaded/downloaded 补算；默认 7 天，上限 90） |

## 实时推送（WebSocket）

| 路径 | 说明 |
|---|---|
| `GET /ws?token=<jwt>` | 实时推送（query token 鉴权，不进 header 鉴权）：种子状态每 2s 广播；H&R 危险种子在 hr-check 发现时即时推 |

消息（按 type 区分）：
- `torrents`：`{ "type":"torrents", "data":[ {"downloader_id":1, "name":"QB", "error":"", "torrents":[Torrent…]}, … ] }`（单下载器失败填 error 不中断）
- `hr-alert`：`{ "type":"hr-alert", "data":[ {"site":"好学", "title":"…", "status":"未达标", "deadline":"10:00:00"}, … ] }`
- `cookie-alert`：`{ "type":"cookie-alert", "data":["好学", "馒头"] }`（cookie 失效站点名列表）

## 注册开放检测（需鉴权）

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/registrations` | 列出监控目标（含上次开放状态/检测时间） |
| POST | `/registrations` | 添加监控目标 `{name, url, signup_path?, open_text?, closed_text?}` |
| PUT/DELETE | `/registrations/:id` | 更新/删除 |
| POST | `/registrations/:id/check` | 手动触发检测 → `{open: bool}` |

## 媒体

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/media/search?keyword=&year=&type=` | 跨源搜索（TMDB/豆瓣/IMDb(OMDb)/Bangumi/MusicBrainz）。`type` 默认 `video`（仅电影+剧集，排除音乐）；可传 `""`全部/`movie`/`tv`/`music` |
| GET | `/media/poster?keyword=&year=` | 首个海报 URL |
| GET | `/media/image?url=` | 海报反代（白名单 CDN，解决豆瓣防盗链） |
| POST | `/media/nfo` | 生成 NFO XML |
| GET | `/media/config` | 源配置（secret 脱敏 + `_configured`；含 OMDb） |
| PUT | `/media/config` | 更新源配置（TMDB/豆瓣/Bangumi/OMDb；空=保留，管理员） |
| GET | `/media/quality` | 多评分订阅质量闸门配置（enabled/min_score/min_votes/min_release_days/sources） |
| PUT | `/media/quality` | 保存质量配置（管理员；清空评分缓存即时生效） |
| GET | `/media/rating?title=&year=&type=` | 多源评分聚合预览：豆瓣/TMDB/IMDb/Bangumi 加权综合评分 + 投票数 + 上映天数 + 闸门判定（`{aggregated,votes,sources:[{source,score,votes}],passed,reason}`） |

> **海报反代**：豆瓣 `doubanio.com` 防盗链，浏览器 `<img>` 直连只回 1×1 占位图。前端对豆瓣图改走 `/media/image?url=`（后端注入 `movie.douban.com` Referer 取真图），且豆瓣 cover_url 会被规整成 `m_ratio_poster`（2:3 竖版）。仅放行 `*.doubanio.com`、`image.tmdb.org`，5MB 上限。**反代客户端用 uTLS 伪装 Chrome 指纹**（`media_fetch.go`）——豆瓣图片 CDN 按 JA3 封 Go 标准 `net/http`（返反爬 JS 页），须 utls 才能取真图。

> **多评分低分保护**（批次5）：`MediaChain.AggregatedRating` 跨源按标题+年份取每源投票数最高的匹配，加权综合（按投票数加权）；闸门三道关——评分 < `min_score` / 投票数 < `min_votes`（样本无统计意义）/ 上映天数 < `min_release_days`（评分未稳定）任一不达标即拦截。订阅 `Check()` 命中种子前查闸门，未通过则本轮跳过推送 + 通知；评分查询失败 **fail-open**（不阻断下载）。全局 `MediaQualityConfig.Enabled` 与单订阅 `enable_rating_guard` 取并集启用。IMDb 走 OMDb（`OMDB_API_KEY`，空则跳过）。聚合结果内存缓存 6h（订阅 cron 高频查不击穿外网）。

## 媒体服务器进阶（批次5）

媒体服务器（Emby/JF/Plex）基础 CRUD/库操作见「下载器」同范式，批次5 新增：

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/mediaserver/:id/playback-report?days=30` | 观影报告：枚举用户→各取最近播放→聚合用户/热门条目/最近播放/总播放数。Emby 走 `/Users/{id}/Items?Filters=IsPlayed`；Plex 历史需 Plex Pass（`/status/sessions` 近似，标 TODO 联调） |
| GET | `/mediaserver/:id/duplicates?library_id=` | 重复媒体检测：枚举库条目，按 TMDB/IMDb ID（优先）或归一化标题+年份归并，返回 `Count>=2` 的组 |
| GET | `/mediaserver/:id/direct/*path` | **302 直链反代（公开）**：开启后把 Emby 文件路径 `{DirectLinkBase}/{path}` 改写到高带宽 CDN，让播放器直连省本机带宽；不泄露 Token（仅静态改写），未开启返 404 |

> **直链反代**对标 MoviePilot Emby 302 直链：播放器无法带 JWT，故端点公开；安全靠「目标 host 来自管理员配置（非请求入参），仅静态拼 `base/path`」，且仅做 302 不注入凭据。服务器配置加 `direct_link_enabled` / `direct_link_base` 两字段。

## 娱乐游戏

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/entertainment/:id/games` | 站点可用游戏列表 |
| POST | `/entertainment/:id/play` | 玩一次 `{game: "jgg"}` |
| GET | `/entertainment/:id/games/:game/stats` | 游戏统计（余额/今日次数/单次消耗，仅 Statable 游戏） |
| POST | `/entertainment/:id/games/:game/play-multi` | 连抽 `{count}`（仅 MultiPlayable 游戏，如九宫格仅接受 10/20/50/100） |
| GET | `/entertainment/:id/games/:game/history` | 抽奖历史记录（仅 Historable 游戏） |

> 能力端点（`stats`/`play-multi`/`history`/`buy`/`prizes`…）按游戏实现的可选接口类型断言分发，不支持的能力返回 400。九宫格（`jgg`）连抽走站点 `/jgg/magic_grid_bulk.php`，历史走 `/jgg.php action=get_history`（`prize_value` 为 PHP serialize，按类型还原 GB/天/个）。

## 文件管理

| 方法 | 路径 | 说明 |
|---|---|---|
| GET/POST/PUT/DELETE | `/storages` | 存储后端 CRUD |
| GET | `/storages/:id/files?path=` | 列目录 |
| POST | `/storages/:id/upload` | 上传文件（multipart） |
| GET | `/storages/:id/download?path=` | 下载文件 |
| POST | `/storages/:id/mkdir?path=` | 建目录 |
| POST | `/storages/:id/delete?path=` | 删除文件/目录 |

**存储类型**（`type` 字段，统一经 `StorageBackend` 接口接入）：
- `webdav` / `alist`：远程网盘协议，需填 `url`/`username`/`password`/`base_path`。
- `smb`：SMB2/3 共享（纯 Go `cloudsoda/go-smb2`，无 CGO），`url`=`smb://主机[:端口]`（默认 445），`base_path`=`/共享名/子目录`（如 `/Media/Movies`）。仅支持 SMB2/3，不支持 SMB1 老 NAS。
- `agent`：通过已部署的 ptpatronus-agent（**0.2.0+**）操作 NAS 本机文件，**无需开 WebDAV/SMB 服务**（push 架构，绕开 SSH/SNMP/SMB 服务依赖，适配飞牛/绿联等不开 SNMP 的国产 NAS）。填 `agent_id`（关联监控 Agent）+ `base_path`（agent 机器上的绝对路径根，深度防御经 `ALLOWED_ROOTS` 白名单二次钳制）。设备离线或 agent 版本过低时文件操作返回错误（提示升级 agent）。
- `115` / `quark` / `123`：网盘只读后端（**仅支持浏览**，写操作返 `ErrReadOnly`）。`password` 存 cookie（115/夸克）或 accessToken（123），`base_path` 为浏览根。云盘原生 client 同 STRM 共用（见下「云盘·STRM」）。

## 云盘·STRM（115/夸克/123）

| 方法 | 路径 | 说明 |
|---|---|---|
| GET/POST | `/cloud/drives` | 网盘 CRUD（`type`=115/quark/123，`token`=cookie/accessToken 明文入参加密落库） |
| PUT/DELETE | `/cloud/drives/:id` | 更新（token 空串=保留）/删除（级联清规则+记录，管理员） |
| POST | `/cloud/drives/:id/test` | 探活（列根目录返回条目数） |
| GET | `/cloud/drives/:id/quota` | 容量 `{used,total}`（best-effort，解析失败返零值） |
| GET | `/cloud/drives/:id/list?path=` | 列目录（UI 文件夹选择器；内部 `resolvePath` 从根 id 逐级解析） |
| GET/POST | `/cloud/rules` | STRM 规则 CRUD（网盘源目录→本地输出目录+模板） |
| PUT/DELETE | `/cloud/rules/:id` | 更新/删除（管理员） |
| POST | `/cloud/rules/:id/generate` | 触发单规则扫描生成 `.strm`，返 `{generated,skipped,errors}` |
| POST | `/cloud/generate-all` | 触发全部启用规则生成 |
| POST | `/cloud/sync-subscriptions` | 订阅追更：已生成 STRM 按文件名匹配订阅→链接+通知 |
| GET | `/cloud/records?drive_id=` | STRM 记录列表（去重+展示，`subscription_id`>0=追更来源） |
| DELETE | `/cloud/records/:id` | 删记录（管理员） |

STRM 文件 = 单行文本指向真实播放路径（默认 `{mount}{cloudpath}`，如 `/CloudDrive2/115/电影/X.mkv`），媒体服务器读取后直链播放云盘资源。生成幂等（本地已存在且内容相同跳过）。`cloud-strm` cron 默认每 6 小时跑 `generate-all` + `sync-subscriptions`。

## 字幕（ASSRT/OpenSubtitles/SubHD/字幕库）

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/subtitle/search?q=&language=` | 跨源聚合搜索（`language` 空=任意 chs/cht/eng）；返回 `{results:[{source,source_id,title,language,lang_code,...}]}`，单源失败不阻断 |
| POST | `/subtitle/download` | 下载字幕（body 为搜索结果项）；解 zip + GBK→UTF-8 + 可选繁简，附件返回字节 |
| GET | `/subtitle/config` | 源配置（secret 脱敏 + `_configured`） |
| PUT | `/subtitle/config` | 更新源配置（ASSRT token / OpenSubtitles key / SubHD·字幕库 cookie / target_lang；空 secret=保留，管理员） |

下载统一管线：解 zip → 编码转 UTF-8 → 按 `target_lang`（chs/cht）繁简转换（仅文本字幕 srt/ass/ssa/vtt，双语/英文不转）。源凭据存 SystemConfig（`subtitle.*` 键），env `ASSRT_TOKEN`/`OPENSUBTITLES_KEY` 提供默认。AI 字幕（faster-whisper 转录）走 `ptp-ai-subtitle` 市场插件，不在本 API。




| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/keepseed/find` | 名称模糊辅种候选 `{name, size}`（Jaccard+大小评分） |
| GET | `/keepseed/config` | IYUU 配置（token 掩码） |
| PUT | `/keepseed/config` | 保存 IYUU token `{iyuu_token}` |
| POST | `/keepseed/reseed` | 触发某下载器 IYUU 精确辅种 `{downloader_id}` → 新增 pending 候选 |
| GET | `/keepseed/tasks?status=` | 列辅种候选任务（pending/added/skipped/failed） |
| POST | `/keepseed/tasks/:id/resolve` | 处理候选 `{action: add\|skip}`（add=添加辅种到下载器） |

## 通知

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/notification/channels` | 已注册通道列表 |
| POST | `/notification/test` | 发送测试通知 |

## 备份

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/backup/export` | 导出配置 JSON（含站点+下载器） |
| POST | `/backup/import` | 导入自身格式 |
| POST | `/backup/import-moviepilot` | 导入 MoviePilot 站点 JSON |
| POST | `/backup/import-ptd` | 导入 PT-Depiler `.ptd` 备份（multipart 文件，自动恢复站点 URL/Cookie/Passkey；加密备份被拒；admin） |

## 系统

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/system/logs` | 运行日志（环形缓冲；`?level=debug\|warn\|error` 筛选、`?n=` 条数，≤5000）|
| DELETE | `/system/logs` | 清空内存日志缓冲（不影响 stdout/落盘）|

## SPA 回退

所有非 `/api/` 路径 → 返回内嵌的 Web UI index.html（vue-router 接管）。
