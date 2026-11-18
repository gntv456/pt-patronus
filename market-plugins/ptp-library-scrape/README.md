# ptp-library-scrape（媒体库刮削）

定时扫描媒体库目录，调用宿主媒体元数据 API（复用 PTPatronus 的 TMDB / 豆瓣等源）补齐 Kodi 兼容的 `movie.nfo` / `tvshow.nfo` 与海报，无需自备 TMDB Key。

## 动作

| 动作 | 说明 |
|---|---|
| `scrape` | 扫描并刮削媒体库 |
| `status` | 查询刮削状态与进度 |
| `search_one` | 预览单个条目的检索结果（input 传 `name`） |

## 主要配置

- `scraper_paths`：待扫描目录，每行一个；可加 `#TV` 或 `#MOVIE` 后缀强制媒体类型，否则按目录结构推断。
- `exclude_paths`：排除目录关键字，每行一个。
- `mode`：覆盖策略（默认跳过已存在 NFO）。
- `media_ext`：视频扩展名列表。
- `cron`：定时任务，留空不自动刮削（如 `0 0 */7 * *`）。

## 权限

需要 `media:search` 权限以调用宿主元数据检索接口。

Former PTPatronus built-in, now open-source market plugin.