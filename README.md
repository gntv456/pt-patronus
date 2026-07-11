# PTPatronus

PT 守护神 —— **干净起点重写**：Go 后端为核心（Docker 部署）+ Vue3 Web UI（Docker 端本体）+ Flutter 客户端（Android/iOS/macOS/Windows/鸿蒙）。

> **当前版本：`0.00.02`（开发版）**。版本号单一真源为后端 `backend/internal/version`，经 `GET /api/v1/system/info` 暴露；前端（设置·关于）与移动端（账户页）**运行时读取后端版本显示**，各自仅保留离线兜底常量。**日常升版只需改后端一处**（`version.go`），重启后端后三端立即生效；`pubspec.yaml` 的 `version:` 是发布构建元数据（Android versionCode / iOS build），与显示版本解耦，仅发布时更新。后端构建可用 `-ldflags "-X .../version.Version=x.y.z"` 覆盖。

> 架构借鉴 MoviePilot（Chain/Module/Event + Oper）、PT-depiler（站点 Schema/Definition 两层 + 286 站 YAML）、pt_mate（SiteAdapter 接口 + CookieCloud）、qBitController（rid 增量同步）。保留「好学 APP 专属 VIP」准入验证。

完整方案见 `C:\Users\52sy\.claude\plans\docker-ios-mac-curious-raven.md`。

## 仓库结构

```
D:\PTPatronus\
├── backend/     # Go 后端（Gin + GORM + SQLite/PG）
├── frontend/    # Vue3 + Element Plus + Tailwind（液态玻璃 Web UI；图表零依赖手绘 SVG/Canvas）
├── mobile/      # Flutter 客户端（含鸿蒙 ohos 说明）
└── docker-compose.yml
```

## 前置

- Go 1.23+（已用 1.26.4）
- Node 18+ + npm（已用 Node 24 / npm 11）
- Flutter 3.41+（鸿蒙需华为 flutter 分支，见 `mobile/harmonyos/README.md`）

Windows 终端中文乱码、端口占用、移动端后端地址等启动问题见 [启动与编码排障](README/启动与编码排障.md)。

> Windows PowerShell 若出现中文乱码，先执行：
> ```powershell
> $OutputEncoding = [Console]::OutputEncoding = [System.Text.UTF8Encoding]::new()
> ```
> 或使用 Windows Terminal / VS Code 终端并设为 UTF-8。

## 运行

### 后端（默认 :8088，SQLite）
```bash
cd backend
go mod tidy
go run ./cmd/ptpatronus
# 或 make dev
```
环境变量见 `backend/configs/config.example.yaml`（SECRET_KEY / ACTIVATION_SECRET 生产必填）。
评审或公开演示实例可显式设置 `DEMO_MODE=true` 显示“浏览演示环境”入口；默认关闭，生产不建议开启。

冒烟（另开终端）：
```bash
curl http://localhost:8088/api/v1/system/health
# 本地初始化（/auth/setup）已停用，登录走好学 APP 专属 VIP：
#   账号登录 /auth/hxpt/login（验证码两步）或 Cookie 登录 /auth/hxpt/cookie-login（覆盖 2FA/挑战响应，最稳）
# 首个通过 VIP 校验的用户自动成为管理员。
curl -X POST http://localhost:8088/api/v1/auth/hxpt/cookie-login \
  -H 'Content-Type: application/json' -d '{"cookie":"<你的好学 cookie>"}'
```

### Web UI（:5173，代理 /api → :8088）
```bash
cd frontend
npm install
npm run dev      # 开发
npm run build    # 构建到 dist/（后端后续 go:embed）
```

### Flutter 客户端
```bash
cd mobile
flutter pub get
flutter run -d windows     # 或 android / macos / chrome
# 真机/模拟器注意 Config.baseUrl（dart-define API_BASE_URL）
```

### Docker（单镜像：API + 内嵌 Web UI）
```bash
make up                 # docker compose 构建并启动（首次会 npm build 前端 → go:embed 内嵌）
# 或直接：
docker compose up -d --build
```
镜像构建流程：前端 `npm build` → 产物 `go:embed` 打进后端二进制 → alpine 运行时。**一个镜像即完整产品**（`:8088` 提供 API + Web UI）。

推 tag 触发 GitHub Actions 构建多架构镜像（amd64/arm64）并推送到 GHCR，详见 `.github/workflows/docker.yml`。

本地验证内嵌效果（不开 vite）：
```bash
make embed-preview      # 前端构建→拷入 backend/internal/web/dist
cd backend && go run ./cmd/ptpatronus   # 访问 http://localhost:8088 即 Web UI
```

可选 PostgreSQL：`docker compose --profile pg up -d`。

## CI / 发布

- `.github/workflows/ci.yml`：push/PR 时跑后端 `go vet/test/build` + 前端 `npm build` + Flutter `analyze/test`。
- `.github/workflows/docker.yml`：打 `v*` tag → 多架构 Docker 镜像 → GHCR。
- `.github/workflows/flutter-release.yml`：打 `v*` tag → Android APK + Windows 产物（artifact）。鸿蒙需华为 flutter 分支 + DevEco，手动构建。

## 当前进度（阶段 0–4 + 三端加固审计已完成 ✅）

> 阶段 0–4 全部交付；其后做了一轮跨三端（后端 / Web / Flutter）的安全与健壮性加固审计，详见文末「加固审计 changelog」。

- ✅ 后端地基：Gin + GORM + SQLite、配置/日志/优雅启停、JWT + bcrypt + AES-GCM、好学 VIP 准入骨架 + 激活码、健康检查路由（编译/运行/鉴权链路已验证）
- ✅ 容器化：多阶段 Dockerfile + docker-compose（可选 PG）+ Makefile
- ✅ Web UI：Vue3 + 液态玻璃主题、登录/首次初始化/VIP 激活、仪表盘指标卡墙 + 全指标趋势图（零依赖 SVG，可切 上传/下载/做种/做种体积/魔力/分享率 + 7/30/90 天区间）
- ✅ Flutter：液态玻璃主题（GlassCard/BackdropFilter）、登录、看板指标卡 + fl_chart（analyze 零警告、测试通过）

### 下载器（阶段 ② 子项，P0+P1 已完成 ✅）

接 qBittorrent / Transmission 官方 API，Web + Flutter 两端同步补强：

- **任务**：列表（rid 增量同步 + WS 实时推送）、添加（magnet/URL **+ .torrent 文件上传**）、暂停/恢复、删除（**可选同时删文件**）、**批量操作**、重新校验、重新汇报、移动保存路径
- **分类/标签**：列出 / 设置 / 增删（qB 零请求取同步状态；TR 用 labels）
- **限速**：单种子 + 全局（统一 bytes/s，TR 内部 KiB/s 换算）
- **文件优先级**（qB 0/1/2/7、TR -1/0/1）、**Tracker** 查看/增/删
- **全局速率面板**：qB 复用 WS 同步状态，零额外请求

后端：`Downloader` interface 扩展 + qb/tr 全实现 + 16 个 REST 端点 + WS stats（`backend/internal/downloader/`）。Web：拆 `frontend/src/components/downloader/` 子组件。Flutter：`file_picker` 选文件（鸿蒙无原生实现时自动 fallback 到链接）。

### 站点保号自动化（阶段 ①，已完成 ✅）

PT 工具核心价值 = 账号不被封。签到防过期、H&R 防警告、cookie 防失效、注册开放抢号——四件套均 cron 自动 + 手动触发双通道：

- **签到**：cron 每天 8:10 自动签到（NexusPHP `attendance.php`，解析「获得 X 魔力 / 今天已签到 / 未登录」）。手动入口：Web 站点行「签到」按钮 + 顶部「一键签到」；Flutter 站点卡片礼物图标 + 顶部一键签到。一键并发签到全部启用站，不支持的站（馒头等走 API key、无传统签到）自动跳过不计失败。**每次签到（自动 + 手动）落库一条签到记录**，Web/Flutter 站点页「签到记录」可回看历史（时间 / 站点 / 结果 / 魔力 / 来源）；历史保留 90 天（`CHECKIN_RECORD_RETAIN_DAYS` 可调），每天 4:30 自动清理超期记录。
- **H&R 告警**：每 30min 抓 H&R 列表 → 判定未达标 / 临期（`HR_WARN_DEADLINE_HOURS` 默认 24h）→ WS `hr-alert` + 通知。
- **cookie 保活**：每 4h 认证 GET 刷新 session；失效则告警「签到 / 统计 / H&R 自动化已停摆」+ 回写轮换后的最新 cookie。覆盖 NexusPHP + unit3d / gazelle / luminance（首页探针，复用各适配器已校准的登录检测）。
- **注册开放检测**：每 6h 探测 `signup.php`，仅在 关闭→开放 转换时通知抢号（去重，不重复打扰）。

均挂 automation cron + capability 接口（`CheckinCapable` / `HRCapable` / `KeepAliveCapable`）：签到 / H&R 目前仅 NexusPHP 实现（unit3d / gazelle 无传统签到与 H&R API）；**cookie 保活**已覆盖 NexusPHP + unit3d / gazelle / luminance。手动触发复用同一 chain 方法，已签识别为「今天已签到」不会重复签到。详见 [04-API参考](README/04-API参考.md) 的「站点管理」段。

后续阶段见方案：① 核心引擎（Chain/Module/Event + 站点适配 + 真实 VIP 抓取）→ ② 下载器 + 自动化 → ③ 媒体/辅种/娱乐/文件 → ④ 打磨发布。

### 可视化工作流引擎与节点库（已完成 ✅）

节点图工作流引擎（借鉴 MoviePilot：节点+边 JSON、拓扑执行、共享 Context、条件分支、每节点循环重试），触发方式：手动 / 定时(cron) / 事件总线。Web=VueFlow、Flutter 自绘画布，**节点库目录与配置表单均由后端 `/workflows/actions` 动态驱动**——加节点零前端改动。节点 **全部封装已存在的 chain/oper 方法**，非空壳。当前 **59 个节点 / 8 类 / 7 个内置模板**：

- **资源(8)**：RSS 获取 / 搜索资源 / 跨站搜索 / 种子详情 / 站点数据 / 排序 / 去重 / 择优
- **过滤(4)**：过滤资源 / 取前 N / 随机抽取（+ 条件分支归逻辑类）
- **下载(16)**：添加下载 / 添加订阅 + **14 个下载器管理**（列出/暂停/恢复/删除/校验/重汇报/强制开始/分类/标签/保存路径/全局与单种限速/分享率限制/添加 Tracker；哈希可由「列出种子」自动承接）
- **站点(11)**：签到 / 全站签到 / 辅种 + 站免 / 公告 / 站内信 / H&R / 保活 / 用户资料 / 连接测试 / 后宫状态
- **媒体(3)**：搜索媒体 / 媒体详情 / 推荐信息流（TMDB / 豆瓣 / Bangumi）
- **文件(3)**：列文件 / 删除文件 / 新建目录（本机 / WebDAV / Alist / SMB / Agent）
- **数据(4)**：HTTP 请求 / 解析字段(正则) / JSON 解析 / 正则提取(列表)
- **通知(3)**：发送通知(站内+外发) / 多通道通知(纯外发) / 发送 Webhook
- **逻辑/流程(7)**：条件判断 / 多分支 / 延时 / 备注 / 设置变量 / 变量运算 / 终止流程 / 格式化字节

代码：`backend/internal/workflow/`（引擎 + 注册表 + 执行器 + 模板）、`backend/internal/workflow/nodes/`（节点实现）。新增节点示例见 `nodes/download_mgmt.go`（`dlMgmtNode` 共用骨架）与 `nodes/site_more.go`（`siteNode` 共用骨架）。

## 客户端导航响应式改造（2026/07/01）

移动端（Flutter，手机 + Windows 桌面共用 `mobile/lib`）主菜单重构，对齐 Docker 端 Web 布局，并复刻重构前（`F:\PT-admin\PTPatronus` Compose `ModalNavigationDrawer`）的侧滑抽屉交互。Docker 端 `frontend/` 未改。

- **移除底部 7 项 NavigationBar**，改为统一的「☰ 主菜单」。
- **响应式**（`kHomeWideBreakpoint = 1000`）：桌面 / 大平板横屏显示常驻左侧玻璃侧栏（对齐 Docker），侧栏可隐藏（右上 `‹`），隐藏后左缘出现 `›` 展开钮；手机 / 窄屏为「最窄 = 手机布局」，仅内容区，☰ 侧滑出抽屉。
- **全量目的地**：抽屉 / 侧栏列出 24 个页面（顺序对齐 Docker Web `navItems`，Flutter 独有的「媒体 / 种子浏览」插入同邻处；末两项「通知设置 / 用户中心」固定底部），原埋在「账户页」里的 14 个二级页面全部提升为一级菜单项。
- **账户页精简**为「用户中心」（登录态 / 版本 / 性能模式 / 登出）。
- 新增 `mobile/lib/features/home/`：`home_controller.dart`（`HomeController` + `HomeShellScope`）、`home_nav_items.dart`（24 项目）、`home_nav_panel.dart`（玻璃面板 + 项 + 品牌头，尊重 `glassReduced`）、`home_drawer.dart`、`home_sidebar.dart`、`home_menu_button.dart`；重写 `home_shell.dart`（响应式 `Scaffold` + 抽屉 + 侧栏，保留通知 FAB）；24 个 nav 页顶栏均加 ☰。`dart analyze lib` 全绿。

> 设计取舍：各页保留各自 AppBar / 标题行（未强删），故桌面侧栏旁仍有页面顶栏；若要更贴近 Docker「单顶栏」观感，可后续移除页 AppBar 并把 actions 迁入页面内工具条。

## Flutter UI 对齐 Web（2026/07/01）

移动端 / 桌面端的 UI、图标、Logo 看齐 Web 端（`frontend/`），消除「两端长得不一样」。`flutter analyze` 零 issue、`flutter test` 31 全绿。

- **彩色导航图标**：新增 `mobile/lib/widgets/glass_icon.dart` + `glyphs.dart`（从 Web `GlassIcon.vue` 的 `GLYPHS` 字典 1:1 移植 24 个 Lucide 线条字形，`flutter_svg` 渲染）。「30% 圆角渐变色块 + 内白边 + 顶部高光 + 外阴影 + 居中白色字形」，逐项 `from/to` 配色与 Web `MainLayout.vue` 完全一致，替换单色 Material 图标。
- **品牌吉祥物 Logo**：新增 `mobile/lib/widgets/pt_logo.dart`（`PTLogo` + `paintPTLogo()`），`CustomPainter` 忠实复刻 `frontend/public/logo-mark.svg`（玻璃盾牌 + 笑脸 + PT 渐变字标 + 彩色尖帽），替换原先的 🛡️ emoji 占位（侧栏品牌头 + 登录页）。不用 flutter_svg：SVG 用了 `feTurbulence` 等滤镜它不支持。
- **导航分组**：`home_nav_items.dart` 由扁平 24 项重构为 4 组（概览 / 内容获取 / 站点活动 / 基础设施），顺序、配色、字形严格对齐 Web；`home_nav_panel.dart` 新增 `NavGroupedList` 渲染组标题（侧栏 + 抽屉共用）。
- **搜索页折 Tab**：`search_page.dart` 加「种子 / 影视」分段切换（对齐 Web `Search.vue`）；「媒体」「种子浏览」从导航折叠进搜索页 Tab（影视复用 `MediaApi` + `MediaPoster` + `MediaDetailPage`），双端导航结构一致。
- **消息分类补全**：`messages_page.dart` 补「作弊者 / 举报信箱」两类，标签与 Web 对齐（收件箱 / 系统 / 发件箱 / 作弊者 / 举报信箱 / 管理组信箱，共 6 类）。
- **全平台启动器图标重生成**：原各平台图标是 Flutter 默认蓝 F。新增 `mobile/tool/render_icon.dart`（`flutter test` 跑 → 画 `paintPTLogo` → 1024² PNG）产出 `assets/icon_source.png`，`flutter_launcher_icons` 一次性覆盖 Android（mipmap-*）/ iOS / macOS / Windows（app_icon.ico）。
- ⚠️ **桌面端需重建**：旧桌面包仍显示 "Nexus Bonus"（源码早是 PTPatronus，是旧包未重建）。`flutter build windows` / macos / android 后重装即恢复。

## 加固审计 changelog（阶段 4 之后，2026/06）

跨三端的安全与健壮性加固，每轮独立 `go test` / `npm build` / `flutter test` 全绿，未破坏既有测试。

### 后端

- **吞错可见化**：`chain/workflow.go`（工作流状态写）、`chain/keepseed.go`（辅种任务状态）、`monitor/alert.go`（告警创建/恢复）原 `_ =` 静默吞掉 DB 写错误 → 改 `slog` 记录。修前工作流会卡 running、告警会丢失（监控失明）、失败辅种任务被当未处理反复重试。
- **输入校验**：`downloader_ops.go` 限速字段 ParseInt 吞错（无效输入静默 = 解除限速）→ `parseLimit` 返 400；`dashboard.go` `?days=` 无上下界（days=999999 触发大查询）→ `clampDays` 收敛。
- **错误脱敏**：`auth.go` 4 处 500 `err.Error()` 泄露 gorm 内部信息 → `dbFail` 通用提示（真实错误进日志）。
- **鉴权收敛**：新增 `middleware/RequireAdmin()`（消费早已注入但从未使用的 `is_admin`），收敛 11 个破坏性端点到 admin：DELETE site/downloader、backup 导入×2、agent create/delete/rotate、webhook create/update/delete/rotate。owner（首个 VIP=admin）与 API-Key 不受影响；非 admin 拿 403 `admin_required`。
- **跨表写事务化**：`oper` 层新增附加式 `WithTx(tx)`（不改任何现有调用），`agent.go` Create 改 `db.Transaction` 真原子（Agent + MonitorTarget 同生共死），去掉手动回滚。
- **保活补齐**：unit3d / gazelle / luminance 实现 `KeepAliveCapable`（共享 `keepAliveIndex`，复用各适配器已校准的首页登录探针，零新增选择器），自动纳入 4h cookie 保活 cron。

### Web（Vue3）

- 破坏性入口对非 admin 隐藏（`v-if="auth.isAdmin"`）：Sites 删站点、Downloaders 删下载器、Backup 两个导入卡、NasServer Agent 按钮、Workflow Webhook 入口。精准对齐后端 RequireAdmin 端点。

### Flutter

- **WsClient 登出泄漏**：登出从不断 WS → 用旧 token 无限重连 + 控制器泄漏；新增 `disconnect()`（`_stopped` 标志 + 可取消 `Timer` 重连，不杀单例，重登可恢复），logout 调它。
- **mounted 守卫**：`monitor_detail _initPlayer`（并 dispose 播放器防泄漏）、`notice _onPush`、`login _fetchCaptcha` 三处 setState 漏判 mounted → 补齐。
- **JSON 解析健壮性**：6 处 `j['id'] as int` 强转 → `(as num?)?.toInt() ?? 0`，单条坏数据不再连累整列表。
- **刮卡逻辑**：`cspt_scratch_view` 自动刮卡 `_playing` 守卫误把"在刮"当"该停"→ 只刮一张就停；改为停止（无卡）/ 跳过（在刮）分开。

### 审计中排除的"非问题"（读代码核实后未改）

- backup 导入事务化：实为追加式 + 重复跳过，非破坏面；事务化会恶化语义。
- webhook 强制 HMAC：token 鉴权 + 可选 HMAC 是合法设计，强制 secret 破坏简单集成。
- 站点级联删除事务化：关联表无 FK，删站只留无害孤儿记录；修需枚举多表级联，风险高于收益。
- message 回复 URL "半 SSRF"：chain 层已有 `sameHost` 守卫。
- HLS token 落 URL：后端 HLS 端点本就是 `?token=jwt` 设计（HLS 无法用 header）。

### 待真机联调（已实现未校准，需真实环境）

MTeam/unit3d/gazelle 的签到 / H&R；消息回复 CSRF 字段名逐站配置；savept.icu 抓取；考核 NexusPHP 跨行聚合；IYUU 真实匹配；真实摄像头 HLS；LongPT 娱乐（每日抽奖 / 幸运大转盘 / 六十四卦勋章）—— `/pt-api/v1/lucky|medals|nexus/*` 契约来自站点 SPA bundle 逆向，抽奖记录字段名 / getCountJoin 返回类型 / 六十四卦 getList 子勋章合成是否真销毁 待真实站点实测校准。NovaHD 三娱乐 + 好学农场（魔力农场 magic_fram.php 好学+NovaHD 通用 magic_farm / 刮刮乐 magic_scratch.php / 幸运抽奖 fortune-wheel-spin.php，契约来自站点源码备份）三端全绿待联调校准：农场 HTML 抠取正则（.points-display/warehouse-table/market-table）、刮刮乐与抽奖的 Stats·Prizes·History 待真实页面校准。OurBits（我堡）菠菜赛事押注（bet.php 列表 / bet_odds.php 盘口 / bet_odds2.php 下单 bonus=&id=选项id 无 csrf，契约实时抓取 ourbits.club）三端全绿待校准：Bet 成功页提示文本结构（当前启发式判定）与 bet_coupon 有押注时的行结构。

## 测试驱动加固（2026/06/30）

一轮「以测试工程师视角」的严格审计 + 修复：基线 `go build/vet/test` 全绿（117+ 测试文件），但发现 **race detector 因 `CGO_ENABLED` 默认关闭从未覆盖并发代码**。审计出 17 处真实缺陷并全部修复，三端回归全绿（后端 test/vet、前端 `npm build`、Flutter `analyze`）。

### P0 严重（越权 / SSRF / 穿越XSS / 数据正确性）

- **越权**：`GET /backup/export` 补 `RequireAdmin`（原 import 三端点收敛但 export 漏网 → VIP 可导出全量明文 cookie/passkey/密码）。
- **SSRF**：workflow `http_request` / `webhook_send` / RSS 抓取由 `ssrf=false` 改 `true`（走 httpclient 私网/元数据防护；闭合 VIP 借 workflow 抓 `169.254.169.254` 并回显响应的链路）。
- **路径穿越**：`agent/safepath.go` + `filemanager/path.go` 清理前 `\`→`/`（POSIX `path.Clean` 不认反斜杠，Windows agent 端经 `filepath.FromSlash` 还原后可 `..\` 上溯越界）。
- **XSS**：`Search.vue` 种子简介 `v-html` 接入 DOMPurify 净化（description 来自 PT 站原始 HTML）。
- **GORM `default:true` 吞 bool**：`Registration/Monitor/Rss` 三处 `Create` 补 `IsActive/Enabled=false` 显式写回（同 `SiteOper.Create` 模式；用户明确停用的注册监控/NAS/RSS 不再被建为启用）。连带修正隐式依赖该 bug 的 `registration_test`。

### P1 高

- `monitors` 组 Create/Update/Delete/Probe/Collect/accept-host-key 收敛 `RequireAdmin`（NAS collector `ssrf=false` 会把账密 POST 到用户指定 host，靠权限收敛；NAS 本需访问内网不能改 ssrf）。
- API-Key 校验改 `subtle.ConstantTimeCompare`（原 `==` 计时侧信道，且命中即授 admin）。
- qb `fetchVersion`/`ge5` 加 `verMu` 锁（version 字段无锁 data race，race detector 因 CGO 关闭漏网）。
- HLS `authHLS` 补 TokenVersion 校验（`RTSPProxy` 注入 `tvCache`；登出/改绑后旧 query token 不再可看摄像头到 exp，对齐 JWTAuth 中间件；WS 走一次性 ticket 不受影响）。
- `.ptd`（ZIP）解压加 `io.LimitReader` + `UncompressedSize64` 预检 + 总量累计（防解压炸弹 OOM）。
- Flutter 账号登录回传 `challenge_mode`（与 Web 端契约对齐）。

### P2 业务正确性

- 考核 `<30% → DANGER` 改 `completed*10 < total*3`（原整数除法在 `total∈{1,2,3}` 时恒假，0% 完成也判 PROGRESS）。
- MTeam `mtIsFree` 改子串匹配（原 `EqualFold("FREE")` 漏 `FREE_2X`/`PersonalFree` 等带修饰免费）。
- NovaHD 抽奖 `isWin` 去掉对 `status` 的依赖（站点省略 status 只回 type+value 时中奖被当未中奖）。
- Gazelle flat-torrent（非分组）分支补 `IsFreeleech` → `Free`（RED/OPS 扁平免费种子原不标 Free）。
- `MonitorOper.Update` 凭据加密失败由静默吞错改为中止返错（对齐 `setCreds`；避免改密码误报成功却仍用旧凭证）。

### 测试流程改进

- **race 覆盖不足（detector 非缺席）**：CI 已有 `go test ./... -race`（`.github/workflows/ci.yml:22`）；qb version race 漏网是因为**没有测试并发触发 `fetchVersion`+`ge5`**，而非 race detector 未运行。本地 Windows 无 gcc 跑不了 race，CI(ubuntu) 能跑。本轮已补 `agent/safepath`（反斜杠穿越）、`filemanager/path`、`oper/registration`（GORM bool 回写）回归测试；建议后续补 qb version 并发测试，让 CI race 能抓到该路径回归。
- 零测试核心包：`internal/agent`（本轮已补 safepath 测试）、`downloader/manager`、`scheduler`、`notice`、`web` —— 建议继续补 oper 层 `default:true` 回写（monitor/rss）回归单测。

### 本轮后续已修 / 仍遗留

- ✅ **Flutter `_toast` mounted 守卫批量补齐**（26 文件 31 处，箭头形式→块形式 + `if (!mounted) return;`，`flutter analyze` 全绿）：异步操作期间导航离开不再触发 `ScaffoldMessenger.of(deactivated context)` 崩溃。
- 仍遗留 P3：Flutter 登录丢弃 `res['user']`（移动端无用户名/admin 徽章，跨 ApiClient+AccountPage）；散点 `setState` 守卫（`await` 后的中间 `setState` 未必全覆盖）；`/system/info` 公开暴露精确版本号；CORS release 模式未收敛。

## 客户端产品测试（2026/07/02）

以产品经理视角对三端客户端做一轮严格测试（可构建性 / 运行冒烟 / API 契约对齐 / 核心页 UX / 三端一致性），发现 **1 P0 + 6 P1 + 9 P2**，已全修，三端回归全绿（后端 `go test ./internal/api/...` + `go build`、Web `vue-tsc --noEmit` + `vite build`、Flutter `analyze` No issues）。

### P0 阻断主流程

- **Web 辅种保存 IYUU token 失效**：`frontend/src/api/reseed.ts` 用 `POST /keepseed/config`，后端只注册 `PUT`，未匹配路由被 `NoRoute` 回退成内嵌 SPA `index.html`（200 HTML），axios 解析失败。改 `POST→PUT`（Flutter 端本就用 PUT，仅 Web 坏）。

### P1

- **后端 NoRoute 系统性陷阱**：`router.go` 原把所有未匹配路径回退 Web UI，导致 `/api/` 下任何错路径/错方法都返 200 HTML 而非 404 JSON，把错误掩盖成「Unexpected token '<'」（P0 长期未被发现即因此）。改为 `/api/` 前缀返 JSON 404，其余才走 SPA。
- **Web typecheck 失败回归**：`BigSmallGame.vue` 对可选字段 `net_result` 未做空值守卫（`vue-tsc` 报 TS18048）。`?? ` 兜底修复。
- **FileManager 上传同名静默覆盖**：补二次确认（删除本就有 confirm，上传反而没有，风险不对称）。
- **Messages 外显后端英文 error**：加 `friendlyError` 映射为中文（cookie 失效 / 超时 / 页面变更等），原始文本保留在 `title`。
- **Settings 通知通道「启用但凭证空」可保存**：加 `validateNotif` 保存前校验（错误原延迟到「发送测试」才暴露）。
- **Flutter 运行日志页缺失**（Web Settings 日志 tab 在 Flutter 全无对应）：新建 `api/system_log.dart` + `features/system/logs_page.dart`（级别过滤 / 刷新 / 清空，仿 announcements）+ 导航 footer 注册。

### P2

- Downloaders 地址 `http(s)://` 格式校验；Login 2FA 6 位数字 + Cookie 含 `=` 校验；FileManager 空态文案 + WebDAV/Alist 类型说明。
- **Flutter 深色主题手动开关**：原仅跟随系统，补三态选择（跟随 / 浅 / 深，`themeModeNotifier` + SharedPreferences 持久化 + 账户页 SegmentedButton）。
- **Flutter 下载器 / 站点编辑契约对齐**：`downloader.dart` / `site.dart` 补 `update` / `test`（仅 api 层；下载器 / 站点页连增删 UI 都还缺，列为独立 backlog）。

### 评估后保留（非缺陷，勿当新问题）

- element-plus 单 chunk 799kB：`vite.config.ts` 已配 `manualChunks` + ElementPlusResolver 按需引入，是库固有体积，gzip 252k + 独立缓存对内部工具可接受。
- 死路由保留：`POST /sites/inspect`（README 推荐的校准工具）、`POST /monitors/:id/accept-host-key`（SSH host-key 确认流程）等。
- 通知通道标签 Bot Token / AppID 等不汉化（PT 圈约定俗成术语，汉化反增歧义）。
- 复测确认：2026/06/30 遗留的「`/system/info` 公开暴露版本号」**已不再成立**——现未登录仅返 `{"name":"PTPatronus"}`，version 需登录（`system.go` 设计如此，安全考虑）。

## Flutter 编辑 UI 补齐（2026/07/02 续）

补上「客户端产品测试」中列为 backlog 的 Flutter 下载器/站点编辑 UI（上轮只补了 api 层契约）。`flutter analyze` 全绿。

- **下载器（downloaders_page.dart）**：标题栏「+」添加；列表项 PopupMenu（编辑 / 测试连接 / 删除）；`_DownloaderFormSheet` 表单（name/type/url/username/password/is_default，编辑态密码留空=不修改）。`Downloader` 模型补 `username` 字段（原 fromJson 漏解析）。
- **站点（sites_page.dart）**：列表项 PopupMenu（编辑 / 删除）；`_showEditDialog`（name / base_url / api_key[仅馒头] / is_active，definition/platform 不可改）；`_deleteSite` 二次确认。
- 站点编辑为窄范围：cookie/username/password 不进表单——Flutter 端 cookie 一贯走 WebView 登录，手填不现实；`SiteApi.update` 现有签名正好覆盖。

## Flutter UI 看齐 Web 收尾（2026/07/02）

承接「Flutter UI 对齐 Web」剩余 6 项重改造，**全部 25 页对齐完成**（自适应 + 内容全对齐）。全量 `flutter analyze` 零 issue。

- **下载器 Downloaders**：纵向 ListView → `PTMasonry` 卡片网格（280px，宽屏多列 / 窄屏单列）；PopupMenu 改 Web 式显式按钮（连接测试 / 任务列表 / 编辑 / **删除**），删除按 `ApiClient().isAdmin` 守卫（对齐后端 `RequireAdmin`）；增删表单走 `showAdaptiveSheet`（宽屏居中对话框 / 窄屏底栏）。
- **监护室 Savept**：站点卡片由 ExpansionTile 内单列 → `LayoutBuilder + Wrap` 响应式网格（**4→3→2→1 断点**，`clamp(1,4)`）；生命体征宽屏改 **ICU 横排带竖分隔线**（窄屏自动换行）；公告区宽屏 2 列。用非滚动 `Wrap` 避免主 ListView 内嵌纵向滚动报错。
- **工作流 Workflow**：补 **webhook admin CRUD UI**——AppBar admin 入口 → 列表（复制接收 URL / 重生成 token / 删除）+ 新建表单（name / 目标工作流 / 可选 HMAC secret）；**编辑器宽屏三栏** `Row(节点库 210 | 画布 | 配置 320)`，复用已抽出的 `NodeParamForm`（`key` 按 nodeId 切换选中强制重建），窄屏保持原 FAB + sheet。
- **文件管理 FileManager**：补**存储后端增删改**——`FileStorageApi` 新增 `create/update/delete` + `FileStorageInput`（后端早有 `POST/PUT/DELETE /storages`）；AppBar「存储管理」菜单（添加 / 编辑当前 / 删除当前）+ `_StorageFormSheet`（类型含 **Agent** + agent 选择器 / 地址凭据 / 根路径，编辑态密码留空=不改）。
- **设备监控 NAS/Monitor**：3 段 ListView → **TabBar**（服务器 / NAS / 摄像头 + 计数 badge）+ TabBarView；卡片**内联 CPU / 内存 / 磁盘迷你进度条**（按 target 取 `metrics?limit=1`，无批量端点、同 Web `loadAll`，单飞 `if(_loading) return` 防 15s 轮询堆积）；**Agent 管理器**（admin sheet：列表 / 新建 / 重置 secret[明文仅一次可复制] / 删除级联）；`Timer.periodic(15s)` 自动轮询。
- **娱乐小游戏**：入口 `_body()` 宽屏套居中 `maxWidth=760` 面板，一处惠及全部 16 个 game view（避免全宽 ListView 在桌面被拉成超宽长条）。

配套 API 层：`api/monitor.dart` 新增 `Agent` 模型 + `AgentApi`（list / create 返 `plain_secret` / remove / rotate，对齐后端 `/monitors/agents`，FileManager 与 NAS 共用）。**发现工作流 `WebhookApi` 客户端早已齐全，仅缺 UI**——先查 API 层省去一半工作。

> 验证范围：静态 `flutter analyze` 全绿。真机 / 桌面宽屏 + 手机窄屏视觉 A/B 与端到端联调（webhook admin、agent 管理器、存储 CRUD、NAS 内联指标）未在本轮做——后端路由 / 契约均已确认存在，建议下轮真机联调校准。

## 修复合集（2026/07/05）

### 后端

- **白板根因——限流误伤静态资源**：全局限流中间件对 `/assets/*.js/*.css` 也计数，浏览器刷新 SPA 并发拉 ~30 个 chunk 瞬间打爆 `GLOBAL_RATE_PER_SEC` 默认 burst=20，后续 chunk 全 429 → Vue 拿不到 JS 崩成整页白屏（「第一次进去正常、再进去/刷新就白板」的根因）。修复：[ratelimit.go](backend/internal/api/middleware/ratelimit.go) `Middleware` 跳过非 `/api/` 路径——限流本就为防 API 暴力破解，静态资源 go:embed 直出、无害。登录限流路径均为 `/api/v1/auth/*`，行为不变。
- **做种数与做种体积同源**：NexusPHP 适配器里做种数（基础页 `color_active`）与做种体积（`getusertorrentlistajax?type=seeding` ajax 翻页）走两条独立路径，且汇总 div「N\|SIZE」只取体积、丢弃数 → 「做种数=0、体积=62.2TB」的自相矛盾。修复：[nexusphp.go](backend/internal/site/schemas/nexusphp.go) `fetchSeeding` 同时返回 `(count, size)`，基础页 `Seeding==0` 时用 ajax 的 count 补齐，数与体积同源。

### Web（Vue3）

- **全局错误边界**：新增 [errorOverlay.ts](frontend/src/utils/errorOverlay.ts) + [main.ts](frontend/src/main.ts) 三路捕获（`app.config.errorHandler` + `window error` + `unhandledrejection`）。Vue 渲染崩溃不再整页白屏，弹可见错误卡片（错误名 + 堆栈 + 刷新/回首页），反馈者能直接截图定位。
- **懒加载 chunk 失败兜底**：[router/index.ts](frontend/src/router/index.ts) `router.onError` 检测 chunk 加载失败（429 / 网络抖动 / 部署后旧 hash）自动刷新一次，不再卡白屏。
- **useWs 登出再登录不重连**：[useWs.ts](frontend/src/composables/useWs.ts) 模块级 `stopped` 单例 `disconnect()` 置 true 后永不重置，`connect()` 入口 `if (ws || stopped) return` 直接跳过 → 通知 / H&R / Cookie 告警不再实时推送。修复：`connect()` 入口重置 `stopped=false`（新会话）+ `await wsTicket` 期间若已 disconnect 则放弃。

### 核查（非缺陷）

- **仪表盘上传/下载是否接反**：全链路核查（后端 NexusPHP 正则锚点 / JSON 字段 / 通用赋值 / JSON tag + 移动端 fromJson / UI 渲染 + Web 端）字段映射全部正确，**无接反**。

> 部署：本次为后端二进制 + 前端打包改动，需重新构建镜像/二进制并重启；部署后触发一次「刷新统计」让做种数补齐写入。
