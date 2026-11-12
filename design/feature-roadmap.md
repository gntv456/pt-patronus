# PTPatronus 功能路线图

> 日期：2026-07-23 · 来源：PT 生态需求盘点 + 源码实证
> 性质：**功能层**规划（非导航/UX，后者见 [menu-redesign.md](menu-redesign.md) v2）

## 一、优化现有功能（高 ROI，契合管理台定位）

### 1.1 刷流删种规则增强 — P0（数据已采集，接入成本低）

> **进度（2026-07-23）**：P0 四项已落地——`DeleteMinUPRateKB` / `DeleteInactiveHours` / `DeleteLogic`+`DeleteRuleExpr` / `MaxTotalSizeGB` 淘汰；qB `LastActivity` 已映射。
- **现状**：[brushflow.go](backend/internal/chain/brushflow.go) `bfMatchRule()` 仅 3 维（DeleteRatio/DeleteSeedHours/DeleteUploadMB），OR 组合。
- **差距**：业界工具有 22 条件 + 布尔表达式 + 9 淘汰策略。[Torrent 结构体](backend/internal/downloader/downloader.go) 已采集 `UPRate`/`NumSeeds`/`NumLeechs`/`NumComplete`/`NumIncomplete`/`Availability` 等，**未接入删种**。
- **P0 补**：① 上传速度下限（`UPRate` 已有）② 最后活跃 `last_activity` ③ 布尔表达式（阶梯策略：新种 ratio>1 删 / 老种 ratio>3 才删）④ 磁盘/总量约束 + 淘汰 action（满时优先删不活跃/慢速）
- **P1 补**：~~swarm 人数 max_seeder/min_leecher~~ ✅ `DeleteMaxSwarmSeeders` / `DeleteMinSwarmLeechers` + 表达式 `swarm_seeders`/`swarm_leechers`；多策略仍可用 DeleteRuleExpr
- **不做**：YAML 配置（DB+UI 更优）、tracker 过滤（用 tag）、多 client（6 适配器更广）

### 1.2 辅种匹配增强 — P1（去 IYUU 单点 + 提升命中率）

> **进度（2026-07-23）**：无 IYUU token 时 `reseedLocal` 走名称+大小跨站搜索（score≥75）；**Torznab** 候选；**IMDb/TMDB ID**（标题提取 + `DownloadMediaLink`）抬分；**体积字节级一致**作数据块强信号（≥92）；文件树 basename 重叠辅助；Torznab 支持 imdbid/tmdbid 查询。完整 .torrent piece hash 比对仍属增强项（需种子元数据）。
- **现状**：IYUU 移植，infohash 跨站匹配，走 IYUU 中心化 API；cookie 下载 .torrent；Reseed 按下载器维度。
- **差距**：本地数据驱动路线用 Torznab（Jackett/Prowlarr）搜候选 + IMDb/TMDB ID + 数据块匹配（**支持重命名/部分匹配**），不依赖中心化服务。
- ① 匹配引入 IMDb/TMDB ID + 数据块（提升重命名场景命中率）② 候选来源支持 Torznab，降低 IYUU 单点依赖 ③ 不改 daemon 模式（保持任务驱动）

### 1.3 字幕自动化 — P2（从手动搜索 → 监听自动）
- **现状**：字幕页手动搜索（ASSRT/OpenSubtitles/SubHD/字幕库）。
- 监听媒体库变化自动匹配下载字幕。

### 1.4 识别引擎 — P2
- 标题解析规则对照查漏（影视名/年份/分辨率/发布组/版本）。

## 二、功能扩张（需拍板，非核心）

### 2.1 发种（Upload）— 不建议核心自建
- 定位冲突（我们是下载后管理 inbound，发种是上传前生产 outbound）；成熟工具多（Upload Assistant/PT-Factory/SmartHash/CRT/GG Bot）。
- 若有需求：**走插件对接**现有发种工具（插件生态+Host API 已就绪），不进核心菜单。

### 2.2 IRC 抢种（Autobrr）— 低优先，优先对接
- 场景：首种/竞速（监听 PT 站 IRC announce 秒抢）。受众窄但上传贡献大。
- 路径：**对接 Autobrr**（省自建 IRC 成本），或低优自建。
- 联动：辅种 + autobrr 可「100% seeder 抢种」（种子发布瞬间以完整数据入场）。

## 三、对接生态（已集成 / 可加强）
- **已集成**：Servarr（Sonarr/Radarr）/ 媒体服务器（Emby/Jellyfin/Plex）/ TMDB / 豆瓣 Frodo / 下载器 6 种（qb/tr/ut/dl/ar/rt/fl）。
- **可加强**：Prowlarr/Jackett 作为**搜索 + 辅种候选**统一后端（标准化 + 去 IYUU 单点）。

## 四、明确不做（形态 / 定位不符）
- content script 注入站点页面（我们是独立应用，不做浏览器扩展形态）。
- seedbox 部署脚本（rtinst/QuickBox/Swizzin）——不涉部署。
- 自建站点 Bonus 积分刷（PTP Bonus Point Optimization 类）——需注入站点，形态不符。

## 优先级速览
| 项 | 档 | 优先 | 成本 |
|---|---|---|---|
| 刷流删种 P0 四项 | 优化 | 高 | 低（数据已在） |
| ~~辅种 ID/数据块匹配 + Torznab~~ ✅ | 优化 | 中 | 中 |
| 字幕自动化 | 优化 | 中 | 中 |
| 发种 | 扩张 | 低（插件） | 高（自建）/低（对接） |
| IRC 抢种 | 扩张 | 低（对接） | 中（自建）/低（对接） |
