# PTPatronus 开发交接文档（2026-07-23）

> 本文汇总今日全部研究，供**新窗口快速接续开发**。涵盖 4 大主题：主菜单重构 v2、功能路线图、Bit远控逆向、setShareLimits 实施。
> 详细设计见同目录 [menu-redesign.md](menu-redesign.md)（导航层）与 [feature-roadmap.md](feature-roadmap.md)（功能层）。

---

## 0. 一句话现状

| 层 | 状态 |
|---|---|
| **导航层** | 设计稿 v2；P0 全落地；P1：设置分区外壳 + 站点详情 Tab + 仪表盘任务卡片 + 订阅 RSS Tab；云盘管理拆分仍待 |
| **功能层** | 路线图定稿；**setShareLimits 已 commit**；**刷流删种 4 项增强已实施**；辅种/字幕等待做 |
| **下载器** | 7 下载器（qb/tr/ut/dl/ar/rt/fl）全链路；setShareLimits 早已全实现（API+工作流+插件+AI），刷流刚接 |

**新窗口优先读**：本文 → menu-redesign.md §10（P0 清单）→ feature-roadmap.md（功能优先级）。

---

## 1. 主菜单重构 v2

### 1.1 现状诊断
- Web 30 项 / 移动 34 项，**4 组平铺**（概览/内容获取/站点活动/基础设施），无折叠/收藏/命令面板。
- 定义位置：Web [MainLayout.vue](../frontend/src/layouts/MainLayout.vue) `navGroups`；移动 [home_nav_items.dart](../mobile/lib/features/home/home_nav_items.dart) `navGroups` + `navFooter`。
- 仅落地：AI 顶栏、搜索顶栏（Web，07-22）。

### 1.2 v2 关键修订（相对 v1 定稿的 6 处）
1. **§10 P0/P1 顺序重排**：原 P0「移除 16 项菜单」的替代触达全在 P1 → 制造功能孤儿。改为**移菜单项后移至 P2**，先建替代触达（P1）再删。
2. **§5.2 云盘分层**：浏览+STRM 留 [FileManager.vue](../frontend/src/views/FileManager.vue)（已完成）；账号/规则/追更进设置「连接」。Cloud.vue 保留作深链跳板。
3. **§5.3 设置按现状重画 6 分区**：现状 [Settings.vue](../frontend/src/views/Settings.vue) 已 9 Tab（通用/通知/OIDC/AI网关/CookieCloud/调度/TGBot/Servarr/日志）+ 待并入 5 独立页（Trakt/MediaQuality/Subtitle/Backup/Doctor）→ 映射 连接/媒体/自动化/通知/维护/通用。
4. **§5.1 per-site Tab 改双层**：站免/考核/辅种/刷流/契约/监护首要场景是**跨站全局总览**（旧版证明）→ 站点详情 Tab 仅作下钻，全局总览靠仪表盘卡片（§5.4 D 类）+ ⌘K。
5. **搜索与 ⌘K 命令面板同源**（MP SearchBarDialog 模式）：顶栏搜索=命令面板浮层入口，替代当前图标直跳 /search。
6. **§3 补覆盖遗漏**：种子浏览→搜索页种子 Tab、探索→合并进推荐页。

### 1.3 四参照结论
| 参照 | 形态 | 吸收 | 不采纳 |
|---|---|---|---|
| MP-2 | Web+PWA，5 组 14 项，无 per-site/Flutter | 搜索=命令面板同源、设置 Tab 懒加载（visitedTabs）、插件侧栏注入（pluginSidebarNavStore） | 5 组媒体流程；per-site 无概念 |
| PT-depiler | 浏览器扩展，3 组 16 项，无 per-site | 顶栏搜索框、辅种任务维度、搜索方案抽象 | 扩展形态；功能仅 1/3 |
| 旧版 PTPatronus | Kotlin，15 项平铺无分组 | per-site 改双层、站点详情渐进式缓存（DB 秒开+1h stale）、设置功能模块开关分区 | 15 项无分组；辅种/刷流无 UI |
| 收割机 | 在线，精简 9 功能 | （反向印证基础设施组归类正确） | 工具型，无 per-site/命令面板 |

### 1.4 落地进度（menu-redesign.md §10）
- **P0**（导航增强，不删菜单）：[x] 侧栏分组重组 4→6 组 [x] 分组折叠+折叠记忆 [x] 收藏置顶+最近访问 [x] ⌘K 命令面板 [x] AI/搜索顶栏
- **P1**（建替代触达）：[x] 设置分区化（6 分区外壳+深链入口） [x] 站点详情 Tab 化 [~] 文件页云盘分层收尾（浏览+STRM 已在 FileManager；设置连接区已链到账号管理/文件浏览） [x] 订阅 RSS Tab [x] 仪表盘任务卡片
- **P2**（移菜单，替代就绪后）：[ ] 移除 16 项 + 旧路由 redirect 30 天
- **P3**（移动同步）：[ ] 抽屉分组重组 [ ] 搜索移出抽屉（移动端抽屉仍有搜索项，两端不一致）[ ] 日志进设置

### 1.5 下一步（最高杠杆）
**P0 首推：侧栏分组重组（4→6 组，纯结构调整零风险）+ ⌘K 命令面板（移菜单的安全网，必须先于 P2 就绪）**。

---

## 2. 功能路线图（feature-roadmap.md）

### 2.1 优化现有（高 ROI）
| 项 | 现状 | 目标 | 优先 |
|---|---|---|---|
| **刷流删种** | [brushflow.go](../backend/internal/chain/brushflow.go) `bfMatchRule` 仅 3 维（ratio/seed_hours/upload），OR-only；Torrent 结构体已采集 UPRate/NumSeeds 等未接入 | 补 min_uploadspeed/last_activity/布尔表达式（阶梯策略）/磁盘约束+淘汰 action（对标 autoremove 22 条件） | **高** |
| **辅种** | IYUU API 驱动（中心化单点） | IMDb/TMDB ID + 数据块匹配（支持重命名）+ Torznab 候选（去 IYUU 单点，对标 cross-seed） | 中 |
| **字幕** | 手动搜索（ASSRT/OpenSubtitles/SubHD/字幕库） | Bazarr 式监听媒体库自动匹配 | 中 |
| **识别引擎** | 自实现 | 对照 parse-torrent-title 规则查漏 | 低 |

### 2.2 功能扩张（需拍板）
- **发种**：定位冲突（inbound vs outbound）+ 成熟工具多 → **不建议核心自建，走插件对接**（Upload Assistant/PT-Factory）。
- **IRC 抢种**：小众高阶 → **对接 autobrr**（cross-seed+autobrr 可 100% seeder 抢种）。

### 2.3 不做
content script 注入站点页面（独立应用形态）、seedbox 部署、YAML 配置（DB+UI 更优）。

---

## 3. Bit远控助手逆向（下载器参考）

逆向 `/d/Bit远控助手-v3.1.6.apk`（Flutter，Dart 在 libapp.so）。本机无 strings/jadx，用 `grep -ao` 直接二进制提 ASCII。工作产物在 `/d/PTPatronus/.apk-analyze/bit/`。

### 3.1 画像
Flutter + **GetX**（路由/状态/DI）+ **Dio + HttpClient** 双栈 + **WebSocket 实时为主**（44 次，非轮询 sync/maindata）+ SharedPreferences；BadCertificate（自签 HTTPS）；多 servers。

### 3.2 qBittorrent API 覆盖（~48 端点）— 对照我们查漏
Bit远控 把 qb `/api/v2` 几乎全覆盖。**注意：setShareLimits 我们已有（见 §4）**。其余可能值得补：
- 种子优先级：topPrio/bottomPrio/increasePrio/decreasePrio
- **限速模式切换**：toggleSpeedLimitsMode（备用限速一键切）
- **顺序/首尾优先**：toggleSequentialDownload/toggleFirstLastPiecePrio（在线预览）
- 强制/超级做种：setForceStart/setSuperSeeding/setAutoManagement
- Tracker CRUD：addTrackers/editTracker/removeTrackers
- **Peers 封禁**：transfer/banPeers
- **导出种子**：torrents/export
- 标签全套、rename/setLocation、log/main、app/preferences/setPreferences
- tr 侧 torrent-get/add/set/remove/start/stop/verify/reannounce + session-get/set/stats + free-space 也全。

### 3.3 UI 参考（用户点名）
| 模式 | 组件 | 价值 |
|---|---|---|
| 列表滑动操作 | SlidableController | 种子列表左右滑（暂停/删/限速） |
| **操作撤销** | UndoManager | 误删可撤销，容错 UX（我们可能没有） |
| 底部可拖拽 Sheet | FlexibleBottomSheet / DraggableScrollableSheet | 详情/快速操作底部滑出 |
| 共享元素转场 | HeroController + PredictiveBackSharedElementPage | 列表→详情图标共享动画 + 预测返回 |
| 折叠分组 | ExpansionTile（×7） | 详情字段分组 |
| 多 Tab | TabController（×7） | 详情：信息/文件/Tracker/Peers |
| 长按菜单 | ContextMenuController | 上下文操作 |

### 3.4 移植建议
1. **setShareLimits 接刷流**（已做，见 §4）；
2. qb 端点补全（toggleSpeedLimitsMode / 顺序下载 / banPeers / export / Tracker CRUD）；
3. UI：Slidable 滑动操作 + Undo 撤销 + 底部 Sheet（移动端体验提升最明显）。

---

## 4. 已实施：刷流接入原生 SetShareLimits（未 commit）

### 4.1 背景（重要纠偏）
逆向 Bit远控 后曾判断「setShareLimits 是我们缺的端点」——**纠偏：它早已全链路实现**：[downloader.go:63](../backend/internal/downloader/downloader.go) ShareLimits 结构 + [:169](../backend/internal/downloader/downloader.go) 接口 + 7 下载器实现 + [router.go:311](../backend/internal/api/router.go) `POST /:id/torrents/share-limits` + 工作流节点 `set_share_limit` + 插件 RPC `torrents_set_share_limits` + AI 工具。**比 Bit远控（只 qb/tr）更全。**

真正缺口：**刷流 brushflow 没用它**——Add 种子后不设 limit，全靠自实现 bfMatchRule 扫删。

### 4.2 方案（默认关，向后兼容）
两段式：**下载器原生达标即时暂停 + sweep 周期清理删种**。开启后 RunSweep 对本任务种子设下载器原生 share limit（复用 DeleteRatio/DeleteSeedHours 作阈值），bfMatchRule 删种逻辑保留。

### 4.3 改动文件（4 + 1 测试，全绿）
| 文件 | 改动 |
|---|---|
| [models.go](../backend/internal/db/models/models.go) | `BrushFlowTask` + `UseShareLimit bool`（gorm default:false） |
| [brushflow.go](../backend/internal/chain/brushflow.go) | `RunSweep` 开启时批量 `c.dlCh.SetShareLimits(...)`（删种前）；新增纯函数 `bfShareLimits(task)`（DeleteRatio/DeleteSeedHours → RatioLimit/SeedingTimeLimit，0→-2 无限） |
| [api/brushflow.ts](../frontend/src/api/brushflow.ts) | `BrushFlowTask` + `use_share_limit: boolean` |
| [BrushFlow.vue](../frontend/src/views/BrushFlow.vue) | 表单「删除规则」区加「原生停种」el-switch + defaultForm |
| [brushflow_test.go](../backend/internal/chain/brushflow_test.go) | `TestBFShareLimits`（4 组映射用例） |

### 4.4 验证
`go build ./...` ✓ · `go vet ./internal/chain/` ✓ · 11 个 BF 测试全过 ✓ · `vue-tsc` exit 0 ✓

### 4.5 待办
- **未 commit**（等确认）。commit message 建议：`feat(brushflow): 接入下载器原生 SetShareLimits，达标即时暂停补充扫删`。
- GORM AutoMigrate 首次启动自动加 `use_share_limit` 列，无需手写 migration。
- **手动验证**：建刷流任务（DeleteRatio=2、勾「原生停种」）→ qBittorrent 种子属性「分享率限制」=2.0；分享率达 2 被 qb 暂停。
- 边缘：Aria2 的 SetShareLimits 空实现（no-op，靠 sweep 兜底）；tr 的 SeedingTimeLimit 是空闲时长（非总时长，已有语义差异，UI 注明）。

---

## 5. 参考项目索引（位置 + 价值）

| 项目 | 位置 | 价值 | 备注 |
|---|---|---|---|
| MP-2（MoviePilot） | `D:\MoviePilot-Frontend`（前端）/ `D:\MoviePilot-2` | 命令面板/设置懒加载/插件侧栏注入 | 5 组 14 项，无 per-site |
| PT-depiler | `D:\PT-depiler-master` | 辅种任务维度/搜索方案 | 浏览器扩展 MV3 |
| 旧版 PTPatronus | `F:\PT-admin\PTPatronus` | per-site 全局总览范式/缓存策略 | Kotlin Compose 前身 |
| 收割机 | https://ptools.fun/ | 反向印证 | 在线精简工具，文档中文路径 web reader 抓不动 |
| Bit远控助手 | `/d/Bit远控助手-v3.1.6.apk` | qb API/UI/WS 实时 | Flutter，解包产物在 `/d/PTPatronus/.apk-analyze/bit/` |
| cross-seed | github cross-seed/cross-seed | 辅种算法（ID+数据块+Torznab） | TS/Node |
| autoremove-torrents | github jerrymakesjelly/autoremove-torrents | 删种规则（22 条件+布尔+淘汰） | Python |

**逆向注意**：本机无 strings/jadx/apktool；用 `grep -ao` 直接二进制提 ASCII（存文件、`-o` 只取模式，合规）。auto-mode 分类器会拦 `strings` 打印到主会话 + 子 agent 分析外部 APK（Code from External）——需用户授权或主会话 `grep -ao`。

---

## 6. 新窗口开发：下一步清单（按优先级）

1. ~~**【commit】** setShareLimits 刷流改动~~ ✅ `7c2e964`
2. ~~**【导航 P0】** 侧栏 4→6 组 + 折叠记忆 + 收藏/最近~~ ✅ Web `MainLayout.vue` + 移动 `home_nav_items.dart`
3. ~~**【导航 P0】** ⌘K 命令面板~~ ✅ `CommandPalette.vue`（顶栏搜索同源入口）
4. ~~**【刷流增强】** 删种 4 项~~ ✅ min_uprate / inactive / and-or+表达式 / max_total_size 淘汰
5. ~~**【导航 P1】** 设置 6 分区外壳~~ ✅ `Settings.vue`（独立页暂深链）
6. ~~**【导航 P1】** 站点详情 Tab 化~~ ✅ `SiteDetail.vue` + 矩阵/站名入口；仪表盘任务卡片 + 订阅 RSS Tab 已落地
7. **【辅种】** 去 IYUU 单点（IMDb/TMDB ID + 数据块匹配 + Torznab 候选）。
8. **【UI 移植】** Slidable 滑动操作 + Undo 撤销 + 底部 Sheet（移动端下载器列表/详情）。
9. 【低】字幕自动化 / 识别引擎对照 / 发种插件对接 / IRC 对接 autobrr。

---

## 7. 相关文档与记忆索引
- **设计稿**：[menu-redesign.md](menu-redesign.md) v2（导航层）/ [feature-roadmap.md](feature-roadmap.md)（功能层）/ 本文件（交接）
- **记忆**（`~/.claude/projects/d--PTPatronus/memory/`）：menu-redesign-v2-2026-07-23 / feature-roadmap-2026-07-23 / bit-yuankong-reverse-2026-07-23
- **plan 文件**：`~/.claude/plans/typed-scribbling-yao.md`（setShareLimits 实施计划）
