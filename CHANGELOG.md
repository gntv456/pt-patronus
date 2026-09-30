# Changelog

所有对外发布版本的用户可读变更记录。格式约定：
- **升级注意**：标注需要用户干预（改配置/改挂载/重登）的破坏性变更
- 完整历史 releaseNotes 见 `update-manifest.json` 各版本条目

## 1.03.22（全链路深度审计加固版，2026-09-30）

本版包含五轮递进式深度审计（安全 → 链路完整性 → 下载器适配 → 并发与资源 →
改动复审 + 运行时黑盒）的全部修复，累计 98 项（P0×7 / P1×26 / P2×58 / P3×7）。

### 安全加固
- STRM 直链签名体系重构：HMAC-SHA256 限时签名（7 天）绑定网盘+路径，签名位置
  修正至行尾（修复签名 URL 畸形导致 302 直链 404 的 P0）；媒体代理校验签名后回源
- 站点同域去重升级为数据库唯一索引兜底（registered_domain 列 + 迁移自动去重回填），
  并发建站不再产生仪表盘上传/做种双计
- 本机 Docker 破坏性操作（删卷/prune/compose down）与站点删除统一要求二次确认
  （confirm_risk），与远程 SSH 版对齐；扩展 popup 修掉一处特权页 XSS 注入点
- SSH 命令输出/容器日志/镜像拉取进度/agent 解压全部加上限（防 `yes` 类命令与
  zip bomb 撑爆内存/磁盘）

### 稳定性（并发与数据完整性）
- 停机链路：WS 广播器（下载完成→媒体整理钩子）与云上传兜底路径纳入优雅停机，
  进程退出不再硬杀在途整理/上传（此前会留半写 NFO/硬链/云端孤儿分片）
- 本机 Docker 客户端修复 Transport 泄漏（此前每次 API 调用净泄漏 1 FD +
  2 goroutine，运维面板轮询数小时后接口批量失败）
- 站点 Cookie/连通性统计改原子列级更新（CookieCloud 同步与手动编辑并发时不再
  互相覆盖丢数据）；删网盘级联清理入事务；工作流防双跑；监控三源并发写串行化
- .strm/备份/compose 文件落盘全部改原子写（崩溃/并发不再产生半截文件被媒体
  服务器扫入库）

### 功能修复
- 移动端：站点矩阵瞬时回显（POST→GET 契约修正，此前一直 404）；401 续期不再
  把写请求静默降级成 GET；VIP 失效专门提示；WS ticket 403 停止无限重连
- 非 qb 下载器（TR/Deluge/Flood/ruTorrent/uTorrent）标签/分类补齐——整理白名单
  不再漏识别；TR 重复添加改幂等命中
- Web 前端：异步竞态守卫（快速切页不再串数据）、双击提交守卫（站点/下载器/
  RSS/云盘/整理按钮）、通知铃铛 WS 实时合并修复、任务轮询失败自动停止

### 升级注意
- 首次启动自动执行 schema 迁移 v3/v4：**同注册域的重复站点会被自动去重
  （保留最先创建的一条）**，日志有 WARN 计数；如有自建镜像/脚本直接 DELETE
  站点的习惯请注意
- Docker 本机模式的删除/清理接口现在要求 confirm_risk（Web UI 已适配；
  自写脚本调用的需在 body 加 `"confirm_risk": true`）
- 直链签名密钥与 SECRET_KEY 同源：更换 SECRET_KEY 会使既有 .strm 内签名失效，
  需到云盘页执行一次「重签全部 STRM」

## 1.03.21（站点数据大修 + 蓝影论坛适配版，2026-09-06）

### 站点数据（Site）
- **魔力/统计抓取大修（参照 PT-depiler 校准）**：青蛙（蝌蚪）、下水道（金币）、
  唐门（星焱）、财神（金元宝）、PT分享站、SpringSunday、ZRPT、ilolicon 等 8 站
  货币改名导致的选择器失效逐站修复；新增 NexusPHP 框架级兜底（color_bonus+[使用]
  锚，与货币名无关）与 userdetails.php 统计行自动合并（魔力/上传/下载/分享率），
  后续站点货币再改名也大概率自动兼容
- PT监护室：详情缓存污染与并发探测数据竞争修复；跨年倒计时站庆不再空窗；
  站名边界误命中（HD→HDHome）修复

### 新功能（Features）
- **蓝影论坛适配**：改版后的蓝影（hdblue.cc，Next.js 自建论坛）接入娱乐中心——
  五子棋人机对战（内置 AI 自动弈完，免费局 +20 影币）+ 阴阳契对赌（智能应战/挂单）
- **守护神名片**：各站上传/下载/做种/魔力聚合数据定时推送到好学论坛
  「PT Patronus 守护神」名片（HMAC 签名，设置页可配）

### 升级注意
- 蓝影 session 绑定登录 IP：换网络后需重新导出 cookie 更新
- 掉线站（PlayLet/SoulVoice/Sunny/慕雪阁/海胆/财神）请更新 cookie 后刷新数据

## 1.03.20（安全加固 + 实时推送稳定性版，2026-09-01）

### 安全（Security）
- **数据库控制台改为只读**：运维中心 → 数据库 的 SQL 查询仅允许 SELECT；
  UPDATE/DELETE/CREATE 等写语句一律 403（曾可通过 confirm_risk 执行任意写 SQL）
- **移除误提交的浏览器 Profile**：`.tmp/ext-profile6`（含 Chrome Cookie/Login Data）
  已从仓库清除。**升级注意**：若你 clone 过旧版本，请重新 clone；曾登录该
  profile 的站点建议重置口令
- JWT 与静态加密密钥 HKDF 域分隔：轮换其中一个不再影响另一个；旧密文自动兼容解密
- JWT 滑动续期加 7 天绝对寿命上限：到期须重新登录（曾可无限自续）
- Agent WS：secret 不再走 URL query（明文 ws 部署下会进反代日志）；
  增加读写超时与应用层心跳，半开连接不再假死 15 分钟
- Agent 一键安装脚本强制 sha256 校验：任一下载源（GitHub/镜像/网盘）被篡改
  即中止安装；agent 升级同样拒绝无校验下载
- Docker compose 默认不再挂载 `docker.sock` 与宿主根目录：需要运维中心
  Docker 管理/文件管理器的用户手动取消注释（详见 compose 内安全提示）
- 浏览器扩展：站点 Cookie 读写改用 optional_host_permissions 动态授权
  （同步白名单时弹窗申请）；移除豆瓣/IMDb 死代码注入脚本
- **扩展敏感凭据加密落盘**：好学账号密码、WebDAV、CookieCloud 密码全部接入
  PBKDF2+AES-GCM 密封存储（原明文自动迁移）；TOTP/凭据库解锁后 10 分钟
  无操作自动清空会话缓存（chrome.alarms）
- release 模式下 CORS 未配置白名单时 fail-closed（与 WS 通道策略对齐）；
  容器 exec 终端 WS 的 Origin 同步收敛（命令执行级能力不再全放行）
- 前端全部 5 处流式请求（SSE/NDJSON/Agent 对话）统一 401 语义：
  会话失效清 user/跳登录并带 redirect，错误文案走 i18n
- GitHub Actions 全部按 commit SHA 固定（16 个 action，防 tag 劫持）；
  trivy-action 从已失效的 0.24.0 tag 升至 v0.36.0
- 新增 CodeQL 静态安全分析（Go + JS/TS，每周 + 主干 push）

### 移动端（Mobile）
- **WebView 登录痕迹清理**：站点/好学 WebView 提取 cookie 上传成功后自动
  清除该站 cookie 与缓存（共用设备不留 HttpOnly 登录态）
- 401 接入 /auth/refresh 滑动续期（后端早已实现，此前直接踢登录页）
- 裸数组响应不再 crash；HTTP 连接池复用 + 幂等请求重试
- Android release 构建签名缺失直接报错（不再静默回退 debug 签名）+ R8 收缩；
  networkSecurityConfig 按域禁公网明文

### 重构（Refactor，行为不变）
- **三端超限文件全量清零（对标「单文件 ~400 行」规范）**：
  - 移动端 34 个 >400 行 .dart 清零（dashboard_page 2331→474、api_client 635→321、
    adaptive 973→340、entertainment 976→233 等，part/extension/mixin 三模式），
    flutter analyze 0 issue、44 测试绿
  - 前端 30+ 个超标 .vue/.ts 清零（TorrentTable 460→368、SiteMatrixTable 480→173、
    Subscriptions 461→336、usePlugins 663→396、OpsDocker 482→374、Cloud 902→345 等，
    拆为子组件/组合式函数/纯工具模块三类），vue-tsc 0 错、build 绿、98 单测绿、
    eslint 0 error
  - 后端 10 个超标 .go 清零（chain/cloud 588→362、auth 480→226、cloud 494→224、
    rousi 525→208、aliyun/quark 上传链路独立文件等），全量 go test 绿
  - **行数门禁升级为三端棘轮**：`scripts/check_file_length.sh` 纳入 mobile/lib
    （.g.dart 豁免），任何非数据文件新增 >400 或登记文件回涨即红；
    baseline 仅剩 i18n locales 翻译表（4160/4121 行，数据阈值 2000 豁免）
- **i18n 翻译表懒加载**：zh-CN / en-US 改动态 import 按需载入——首屏只载用户
  当前语言一张（index chunk 264KB → 24KB，减约 100KB gzip），切换语言时再取
  另一张；载入前 t() 走兜底文案不白屏，挂载等表就绪保证首帧完整
- **zh/en 翻译表 key 一致性测试**：两表递归展开为点分 key 全集做双向 diff +
  结构错位检查——新增文案只改一张表时 CI 红（此前英文用户会静默回落中文）
- **移动端巨型文件拆分**：dashboard_page.dart 2998 → 2331 行，抽出
  TotalDashboardConfig 模型 / 设置面板 / 分享图模型 / 历史数据浮层四个文件
- **浏览器扩展 options.js 拆分**：1535 → 1080 行，站点数据 / 下载任务 /
  两步验证器三个页面模块化（新文件纳入 innerHTML XSS 静态审计门禁）
- **OpsDocker.vue 拆分**：750 → 548 行，数据刷新/监控轮询与容器动作/批量
  操作分别抽为 useDockerData / useContainerActions 组合式函数
- **移动端框架组件文案语言固定 zh-CN**（flutter_localizations）：非中文系统
  下日期选择器等 Material 组件不再与中文业务文案混杂

### 修复（Fixed）
- **WS 增量协议丢帧自愈**：慢消费者丢一帧 diff 后原需等 30 帧周期 full
  重同步（约 60s）才能恢复，期间任务列表静默停滞——现在丢帧即主动断开
  重连（防抖 1s、跳过退避），由 Serve 首帧的 torrents_full 立即对齐；
  任务抽屉在 WS 不可达窗口内增加 REST 兜底拉取（打开即拉一次）
- GORM `ErrRecordNotFound` 判定 3 处改用 `errors.Is`（转移历史/考核/勋章
  not-found 误报为真实错误的缺陷）
- 媒体代理无超时挂起：补 60s 总超时 + 30s 响应头超时
- 前端 http 层错误文案接入 i18n（英文界面不再混中文）；401 处理补
  redirect 回跳、并发去重、user 快照清理；SSE 流 401 与拦截器同语义
- llm_gateway/notification/servarr 5 处内部错误信息泄漏收敛

### 数据 / 可观测性（Data & Ops）
- RSS 条目与插件日志补 30 天 TTL 清理（原先无限膨胀）
- 新增 readiness 探针 `GET /api/v1/system/health/ready`（DB 连通检查，
  不可用返 503；liveness `/health` 行为不变）
- 日志级别支持 `LOG_LEVEL` 环境变量（debug|info|warn|error）
- **备份导出加密**：导出时可设口令（PBKDF2-SHA256×310k + AES-256-GCM 信封），
  导入自动识别加密备份并提示输口令；旧明文备份兼容导入

### 可访问性（Accessibility）
- **全站可点击 div 补齐键盘可达**：24 个组件/页面 37 处 `<div @click>` 接入
  `v-a11y-button` 全局指令（role=button + tabindex=0 + Enter/Space 触发 click），
  键盘与读屏用户此前完全无法操作的卡片/列表项全部可用

### 测试（Testing）
- E2E 从 3 条扩到 8 条：登录失败、登出（TokenVersion 失效链路）、
  i18n 切换持久化、订阅建档-列表-UI 删除（含确认框）
- **修复 mock qB 的 rid 协议 bug**：曾用 torrents.length 当 rid，添加种子后
  长度恰好等于上一轮返回值时被误判"无增量"→ 任务永不出现（全量套件必现、
  单跑碰不到的经典顺序依赖）；改为状态变化单调 +1，对齐真实 qB 语义
- 数据库迁移升级为版本化框架：`schema_migrations` 表 + 按序执行恰好一次 +
  失败事务回滚；历史订阅布尔回填收编为迁移 0001（老库 SystemConfig 标记兼容），
  新增 4 个迁移行为测试（幂等/兼容/版本递增/失败原子性）
- 首批 Fuzz 测试落地（ParseBytes / ParseClaimPair，281 万+ 执行 0 crash）；
  **顺带修复真实溢出**：`ParseBytes("999… TB")` 超大数值经 float64×单位
  换算转 int64 翻转为负数（体积显示为负、做差异常），现钳制到 MaxInt64；
  附 ParseBytes / SumBytes 基准（~3.1µs / ~4.5µs 留档对比）
- 前端 lint 升级：ESLint 9 flat config 接入 @typescript-eslint（no-explicit-any
  等 5 条 warn）+ Prettier（printWidth 120），存量 0 error、231 warn 渐进清零；
  `npm run format` / `format:check` 一键格式化

### CI/CD
- CI 加并发去重（同分支新 push 取消旧运行）、全 job 超时、最小权限声明
- 新增 Dependabot（gomod/npm/pub/docker/actions 每周依赖更新 PR）
- Agent 二进制回归 CI 构建发布（此前为本机构建，供应链不可审计）
- update-manifest 校验新增：PENDING 占位符拦截 + 多端同 URL 拦截已生效
- 鸿蒙构建 chewie 本地 checkout 路径参数化（-ChewieOhosPath /
  PTP_CHEWIE_OHOS_PATH），换机器不再需要手改仓库文件
- 镜像供应链收尾：docker build-push-action 开启 SBOM + provenance(mode=max)
  输出，cosign keyless 签名（digest 级），Trivy 扫描不再限 tag 构建——
  dev 镜像同样被扫；`make test-frontend` 实跑 npm test + typecheck，
  sdk-release 发布前先过 npm test（发布门禁前移）

### 文档（Docs）
- 新增《快速开始》用户指南（取 Cookie 图解步骤、FAQ、反馈渠道）

## 1.03.19

浏览器扩展改为好学账号登录（与 Web/移动端同一账号体系）；JWT Bearer +
/auth/refresh 滑动续期；插件 2.6.0 Cookie 自动同步保活闭环。详见 manifest。

## 1.03.18

云盘 STRM 一条龙补齐 + 自动化链路修复；补鸿蒙 hap + iOS ipa 发布产物。

（更早版本见 update-manifest.json 历史 releaseNotes）
