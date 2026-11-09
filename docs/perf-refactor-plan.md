# 性能与流畅重构方案（推荐稿）

> 目标：在**只考虑性能和流畅**的前提下，给出 PTPatronus 的可执行重构路线。
> 现状基线：Go 1.26 后端（~9 万行 / 47 业务域）+ Vue3 Web UI（47 views / 171 组件 / 304 站点 YAML）+ Flutter 三端 + MV3 扩展 + `go:embed` 单镜像。

---

## 一、拍板结论

**不做推倒式全量重写，做「保内核、换渲染层、旁路热路径」的渐进式重构（Strangler 模式）。**

三条决策依据：

1. **瓶颈不在语言**。前两轮排查已确认：全站 187 处 `backdrop-filter`、零虚拟列表、15 处 `setInterval`、SVG 图表、Element Plus 组件树、GORM+SQLite 聚合——换 Go→Rust 或 Vue→Solid 单个都解决不了这些。
2. **微内核不是性能方案**。它买的是隔离/可扩展/故障域，付的是间接分派与消息总线。而项目现有的「编译期插件化 + 运行时单体」形态（内置 Module + YAML 钩子 + 单二进制 + `go:embed`）恰恰是性能最优的一种微内核，**不要改成动态加载 / 多进程 / 微前端**。
3. **收益排序**：数据流与渲染策略 ≫ UI 框架 ≫ 语言/运行时。所以阶段 1（不换任何框架）必须先行，并设 go/no-go 门槛。

**最重要的一个决策点**：阶段 1 完成后若指标全部达标，**就到此为止，不换框架**，可省下 6–10 人周和一次全量迁移风险。

---

## 二、最终技术栈（定稿）

| 层 | 定稿 | 说明 |
|---|---|---|
| 后端语言/HTTP | **Go 1.26 + Gin 保留** | 不动 9 万行业务代码 |
| 业务编排 | `Chain / Module / Event` **保留在冷路径** | 站点抓取、签到、通知、工作流、插件市场 |
| 热路径 | 直连 fast path：定长 struct + ring buffer | WS 增量 diff、下载器 rid 同步、监控指标、仪表盘聚合 |
| 数据层 | 热路径 SQL 用 **sqlc**；GORM 留给冷路径 | SQLite 开 WAL + 批量写 |
| 时序/聚合 | 指标快照迁 **DuckDB**（嵌入式列式）+ 内存快照 + 定时 rollup | 先双写、只读旁路验证后切读 |
| WS 协议 | CBOR/MessagePack 二进制，复用 `?proto=` 协商位双协议并行 | 现有 `?proto=inc` 机制扩展，老客户端不变 |
| Web 框架 | **Vue 3 继续**（阶段 1–2）；仅指标不达标时试点 **SolidJS** 逐页 strangler 迁移 | **不押注 Vue 3.6 Vapor（RC）** |
| 状态 | 引入 `alien-signals`（仓库已有间接依赖）做 per-entity 细粒度 store | 逐步替代 `torrents.value` 整块 ref 赋值，`applyDiff` 纯函数与单测保留 |
| UI 组件 | **新页面禁用 Element Plus**；自研 15–20 个 headless 原语；旧页面不动 | 禁止一次性全量替换 |
| 样式 | Tailwind 保留；玻璃改静态渐变，全站默认关闭 blur，仅保留 ≤3 处合成层 | Tailwind 零运行时 |
| 大列表 | 统一封装 `VirtualList.vue`（`@tanstack/virtual`）+ `content-visibility` | 5 个热点页面优先 |
| 图表 | **uPlot / Canvas 2D** + OffscreenCanvas in Worker；SVG 仅留给 `GlassIcon.vue` | 上千数据点不再产生上千 DOM 节点 |
| 计算 | `data.worker.ts`（解析/排序/聚合/图表）+ Comlink | 主线程只留合成与交互 |
| 实时合帧 | `useWs.ts` 的 `onmessage` 加 `requestAnimationFrame` 缓冲队列 + 按 key 合并 | 轮询统一调度器，页面隐藏时暂停 |
| 构建 | Vite 8 / Rolldown（稳定后升）+ Brotli 预压缩 | 路由级懒加载**已做，保留** |
| 客户端 | **Flutter 完全不动** | 只验证 Impeller、关 Web 端 blur；AOT 已是流畅第一梯队 |
| 插件 | 编译期：`go:generate` 把 304 YAML 生成 Go 常量；运行时扩展一律 Worker/WASM 隔离 | 微内核只强化隔离，不做动态化 |

### 明确不做（Negative List，防范围蔓延）

- ❌ Go → Rust 重写后端
- ❌ 微前端 / 模块联邦 / 运行时 UI 插件注册
- ❌ 动态插件加载 / 多进程微内核（会把 WS 推进跨进程序列化，P99 恶化）
- ❌ Electron / Tauri 桌面壳
- ❌ Flutter 重写或动态下发 Dart 代码
- ❌ 一次性替换全部 Element Plus
- ❌ Vue 3.6 Vapor Mode 上生产（2026-10 仍为 RC）
- ❌ 移动三端与 Web 同时改动

---

## 三、现状盘点：可直接复用的钩子

| 已具备 | 位置 | 用法 |
|---|---|---|
| 路由全量懒加载 | `frontend/src/router/index.ts` | 保留，不要重复造首屏分包 |
| WS 增量 diff + seq/丢帧重同步 | `frontend/src/composables/useWs.ts` | `applyDiff` 纯函数与单测保留，只在上层加合帧与细粒度订阅 |
| WS 协议协商位 | `useWs.ts` 的 `?proto=inc` | 扩展为 `?proto=cbor`，做二进制协议灰度 |
| 热路径已手工绕开反射 | `backend/internal/ws/broadcaster_diff.go:45` | 快慢路径分离的既有范例，沿此思路推广 |
| CI 棘轮基线模式 | `scripts/file_length_baseline.txt` + `scripts/check_file_length.sh` + `.github/workflows/ci.yml` | **照抄该模式**做 `perf_baseline.json` + `check_perf.sh` + `perf-gate` job |
| 站点定义同步脚本 | `scripts/sync-site-defs.sh` | 旁边挂 `go:generate` 生成 Go 常量 |
| Makefile 统一入口 | `Makefile` | 加 `perf` target |
| 单测底盘 | 后端 1844 个测试 / vitest / Playwright / `-race` / coverage floors | 重构安全网 |

**当前缺失**：Web Worker（0 处）、`go:generate`（0 处）、性能基线与门禁（0 处）、虚拟列表（0 处）。

---

## 四、验收指标（阶段 0 建档，每阶段只认数字）

| 指标 | 场景 | 目标 |
|---|---|---|
| LCP | 冷启动首屏 | < 2.5s |
| INP (P75) | 仪表盘 / 列表交互 | < 200ms |
| 帧时间 (P95) | 1000 条种子列表滚动 | < 20ms（≥55fps） |
| 长任务 >50ms | WS 1000 msg/s 压测 | 0 |
| 首屏 JS (gzip) | 主 chunk / 总量 | < 150KB / < 300KB |
| Lighthouse Performance | 移动端节流 | ≥ 90 |
| WS 广播 P99 | 后端 | < 5ms |
| `/dashboard` P99 | 后端 | < 100ms |
| 稳态内存 | Web 页面 | < 150MB |

基线采集 5 个固定场景：① 冷启动首屏 ② 仪表盘 7/30/90 天切换 ③ 1000 条种子列表滚动 ④ WS 高频推送 ⑤ 摄像头/终端面板打开。要求 CI 复现波动 < 10%。

---

## 五、分阶段路线图

### 阶段 0 · 可度量基线（1 周，全员，gate 所有后续）

- 新建 `docs/perf-baseline.md`、`scripts/perf_baseline.json`、`scripts/check_perf.sh`，照抄 `file_length_baseline.txt` 的棘轮模式。
- `Makefile` 加 `perf` target：Lighthouse CI + Playwright 帧时间录制 + 后端 `go test -bench` / pprof（`/dashboard`、WS 广播）。
- `.github/workflows/ci.yml` 加 `perf-gate` job：**前 2 周只报告不阻断**，稳定后转阻断。
- **验收**：5 个场景数值入档且 CI 可稳定复现。**不达标不进阶段 1**（先修采集脚本）。

### 阶段 1 · 零换栈攻坚（3–4 周，前端 2 人 + 后端 0.5 人，预期拿 70–80% 收益）

按 ROI 排序，**不动 Vue / Go 框架 / Flutter / Element Plus 存量**：

| # | 动作 | 落点 |
|---|---|---|
| 1 | `glass-reduced` 改默认开启 + 设置项可关；187 处 blur 改静态渐变，仅保留 ≤3 处合成层 | `frontend/src/main.ts`、`frontend/src/styles/glass.css` |
| 2 | 新增 `components/common/VirtualList.vue`，覆盖 5 个热点 | `components/downloader/TorrentTable.vue`、`TorrentDrawer.vue`、`views/Sites.vue`、`components/NotificationCenter.vue`、插件/日志页 |
| 3 | 15 处 `setInterval` 收敛到 `composables/usePolling.ts` 统一调度器（可见性感知 + 隐藏暂停 + 合并同 tick）；高频项改由 WS 推送 | `OpsMonitor.vue:85`(3s)、`DashboardQuickTools.vue:72`(1s)、`TorrentDrawer.vue:183`(5s) 优先 |
| 4 | WS 合帧：`onmessage` 加 `rAF` 缓冲队列 + 按 key 合并；`taskProgress`/`notices` 改 `shallowRef` + 精确 patch | `frontend/src/composables/useWs.ts` |
| 5 | 细粒度订阅：`alien-signals` per-torrent store，把整块 `torrents.value` 赋值降级为按 hash 更新 | 同上，`applyDiff` 纯函数与单测不动 |
| 6 | 趋势图换 uPlot（Canvas 2D），SVG 只留图标 | `components/dashboard/*`、`components/GlassIcon.vue` 保留 |
| 7 | Element Plus **仅新页面禁用**，旧页面不动；`main.ts` 的 4 处手动 CSS 补丁与 dark css-vars 保留 | `frontend/src/main.ts` |
| 8 | Vite 8 / Rolldown 升级 + `go:embed` 侧 Brotli 预压缩；路由懒加载保留 | `frontend/vite.config.ts`、`Dockerfile` |

- **验收**：指标表全绿。
- **Go/No-Go（核心）**：全绿 → **方案结束，不进阶段 2**；未全绿 → 定位剩余短板，进阶段 2 试点。
- **回退**：每项一个 feature flag，逐页灰度。

### 阶段 2 · Web 渲染层试点（仅当阶段 1 未达标；2 周试点 + 4–6 周推广，前端 1–2 人）

- 选 `views/Downloaders.vue` + `TorrentTable.vue`（最痛、边界清晰）用 **SolidJS** 重写为**同 bundle 内的独立挂载点**（共享 WS store，**不是微前端**）。
- 用阶段 0 同一场景 A/B 对比；**提升 < 15% 即停止**，退回阶段 1 深挖。
- 达标后按「高频实时页面优先」逐页 strangler 迁移：Dashboard → Downloaders → OpsCenter → 其余 views。每页一个开关，可回滚。
- Vue 3.6 Vapor 仅做技术预研，不进生产。

### 阶段 3 · 后端热路径（与阶段 2 并行，3–4 周，后端 1 人，独立流水线）

- **快慢分离**：`Chain/Module/Event` 留给冷路径；`backend/internal/ws/broadcaster.go` + `broadcaster_diff.go` 走直连（定长 struct + ring buffer），热路径去掉 `map[string]any` 与 `Publish` 的 `wg.Wait()` 等待语义（尾延迟不再由最慢订阅者决定）。
- **协议灰度**：复用 `?proto=` 协商位加 `?proto=cbor`，双协议并行 2 个版本，老客户端不变。
- **数据层**：`/dashboard` 改内存快照 + 定时 rollup；指标时序迁 DuckDB（先双写、只读旁路验证后切读）；热路径 SQL 用 sqlc 生成。
- **验收**：WS 广播 P99 < 5ms、`/dashboard` P99 < 100ms、1000 msg/s 下主线程零长任务。

### 阶段 4 · 编译期插件化（2 周，依赖阶段 3 稳定）

- `scripts/sync-site-defs.sh` 旁新增 `go:generate`：304 个 YAML → 生成 Go 常量/选择器表，运行时零解析。
- 前端「插件」放构建期：条件导出 + tree-shaking；`market-plugins` 的运行时扩展一律塞 Worker/WASM 隔离。
- 微内核只强化隔离：manifest 版本化、超时熔断、崩溃域；**不做动态加载、不拆多进程**。
- Flutter 三端零改动。

---

## 六、并行关系与人力

```
阶段0(1周,全员) ──> 阶段1(3-4周, 前端2 + 后端0.5) ──> [Go/No-Go]
                                                    ├─ 达标 ─> 结束
                                                    └─ 未达标 ─> 阶段2(前端) ║ 阶段3(后端) ──> 阶段4
```

- 阶段 2 与阶段 3 **完全并行**，共用阶段 0 的 perf 门禁，互不阻塞。
- 全程 Flutter 0 人力；后端在阶段 1 只需配合 WS 帧格式约定。
- 7×24 常驻服务要求：每阶段一个 feature flag，逐页/逐端点灰度，可秒级回滚。

---

## 七、风险登记

| 风险 | 缓解 |
|---|---|
| 二进制 WS 前后端版本耦合 | 复用 `?proto=` 协商，双协议并行 2 个版本 + 灰度开关 |
| Element Plus 迁移大爆炸 | 新页面禁用、旧页面不动，禁止全量替换 |
| DuckDB 引入 | 先双写 + 只读旁路，验证后切读，保留回退开关 |
| 玻璃降级视觉回归 | 设置项默认开、可关；设计侧先确认静态渐变稿 |
| 性能收益无法证明 | 阶段 0 门禁先落地，每阶段只认数字 |
| 换框架后生态/人力成本 | 阶段 1 设硬性 go/no-go，默认不换 |

---

## 八、第一周就能开工的 3 件事

1. 落地 `make perf` + `scripts/perf_baseline.json`（照抄棘轮模式），把 5 个场景数字入档并跑通 CI。
2. 把 `main.ts` 的 `glass-reduced` 从「仅低端机」改为**默认开启 + 设置项可关**，用同一场景 A/B 看帧时间。
3. 给 `TorrentTable.vue` 套上 `VirtualList.vue`，对比 DOM 节点数与帧时间（这通常是单点收益最大的一改）。

> 三件事都不改框架、不改架构、可灰度、可回滚，且能立刻用阶段 0 的数字证明收益。

---

## 九、实施状态（本轮开工落地）

| 阶段 | 交付物 | 状态 |
|---|---|---|
| 0 基线 | `scripts/perf_baseline.json` / `check_perf.mjs` / `collect-perf.mjs` / `docs/perf-baseline.md`、`make perf*`、CI `perf-gate` | ✅ 已跑通 |
| 1.1 玻璃降级 | `composables/usePerfMode.ts` + `main.ts` 默认 `glass-reduced`、设置项 `setGlassBlur()` | ✅ |
| 1.2 虚拟列表 | `components/common/VirtualList.vue` + 4 个单测（`computeRange` 纯函数） | ⚠️ 组件已交付，**未接入**：现有列表为 Element Plus 表格（自带分页）或变高通知卡片，定高替换会造成视觉回归，待配变高测量后接入 |
| 1.3 轮询统一 | `composables/usePolling.ts`（单 1s tick / visibilitychange 暂停 / 切回补跑），接入 OpsMonitor(3s)、QuickTools(1s) | ✅ 2 个热点已迁移；**TorrentDrawer(5s×2) 保留原实现**——其 8 个时序敏感单测依赖精确 `setInterval` 节奏，迁移后挂测，已回退并加 `document.hidden` 短路；低频 30s/60s 与游戏动画计时器保留 |
| 1.4 WS 合帧 | `useWs.ts`：`rAF` 帧队列 + `shallowRef` 细粒度替换，`applyDiff`/seq/重同步/全部单测保留 | ✅ |
| 1.5 Canvas 图表 | `Sparkline.vue` 内部改 Canvas 2D（props/class/tooltip 契约不变，所有引用零改动） | ✅ |
| 1.6 构建 | `vite.config.ts` 细粒度 chunk + `modulePreload.polyfill:false`、**EP 不再整体合并成单一 241KB chunk（首屏 -61.8%）**、`scripts/analyze-bundle.mjs`、`scripts/check-no-new-ep.mjs` 棘轮门禁 + CI | ✅ 首屏达标 |
| 2 换框架 | SolidJS 试点（`/perf-lab` 独立 A/B 页，零改动现有组件/测试） | ❌ **NO-GO**：高压档每帧更新+DOM diff 耗时 Vue 3.1ms vs Solid 7.2ms（Solid 慢 132%），两档帧间隔均 16.8ms；按 ≥15% 判据停止推广 |
| 3 后端快路径 | `event.PublishAsync`（去 N goroutine + wg.Wait + map 拷贝）、`ws.Batch`（合并锁获取，`PTP_WS_BATCH=0` 回退）+ 单测 | ✅ |
| 4 编译期插件化 | `scripts/gen-site-index.mjs` → `zz_defs_index.go`（307 定义 / 438 host）、`site.MatchByURL` 快速路径（`PTP_SITE_FASTPATH=0` 回退）、`make gen-site-index`、`go:generate` | ✅ |

### 实测数字（阶段 0 采集器，静态口径）

- 演变：首屏 **557.3 KB**（初始）→ **403.5 KB**（阶段 1.6 细粒度 chunk）→ **154.3 KB**（本轮 EP 分块，阈值 300 ✅）；主 chunk 241.6 → **65 KB**（阈值 150 ✅）；全量 1026.6 KB。
- 根因与解法：`App.vue` 根组件静态用了 `ElConfigProvider`，而 `manualChunks` 把全部 `element-plus` 强行合并成单一 241KB chunk 并被首屏 modulepreload。改为让 EP 模块交回 Rollup、按引用它的懒加载路由自动分块后，首屏只留 `vendor` 65 + `vue` 47.8 + `index` 15.6 + CSS/icons。**没有删除任何 EP 组件、没有换框架、没有改业务代码。**
- 当前门禁：**4 PASS / 0 不达标**（首屏 154.3 < 300、主 chunk 65 < 150、帧时间 P95 16.8 < 20、稳态堆 3.8MB < 150；帧时间在 16.7–16.8ms 间采样波动）。
- 冒烟：Playwright 打开登录页完整渲染，2 条 console 错误均为「后端未启动」的 API 500，**无 pageerror / 无 chunk 加载失败**，分块改动安全。
- 结论闭环：首屏问题由打包策略解决，印证「瓶颈在渲染 / 数据流 / 打包，不在框架」；阶段 2 换框架的 NO-GO 成立。
- 仍未采集：LCP / INP / Lighthouse / `long_tasks`（挂载期）标 `MISSING`，不算门禁失败——需真实浏览器交互 + Lighthouse CI。WS 广播 P99 与 dashboard P99 已用脚本/bench 补齐（见下）。


### 阶段 2 试点结论（NO-GO）

- 门禁首次实测：`frame_p95` 16.7ms PASS；首屏 403.5KB / 主 chunk 241.6KB FAIL → 触发 go/no-go，进阶段 2。
- 独立试点页 `/perf-lab`（顶层路由、避开鉴权守卫、**不动任何现有组件/测试**）：Vue 快照整体替换（现状语义）vs Solid `createStore` 行级细粒度更新，4000 行 / 每帧改 250 行 / 90 帧 / 交替各两轮取均值。
- 标准档（1000 行 / 50 更新）：帧间隔 Vue 16.8ms vs Solid 16.8ms，提升 0%——rAF 间隔被 vsync 钳在 16.7ms，负载未超一帧时测不出差异。
- 高压档改测「每帧更新 + DOM diff 同步耗时」：**Vue 3.1ms vs Solid 7.2ms，Solid 慢 132%**（`costGainPct = -132.3`）。
- 机理：Solid 每次 `setState` 是独立细粒度更新，250 次/帧的批量更新下累计开销高于 Vue 一次性 keyed diff；本项目 WS 增量恰是「一批 changed 行」，不是单行高频。
- 判定：**停止全量换框架**，按方案回阶段 1 深挖——首屏 241.6KB 的 Element Plus 才是静态可测的不达标根因。
- NO-GO 后已清理：`npm uninstall solid-js vite-plugin-solid`（移除 38 包）、删除 `frontend/src/perf-lab/`、`frontend/src/views/PerfLab.vue`、`scripts/perf-pilot.mjs`、路由 `perf-lab` 条目、守卫白名单与 `vite.config.ts` 的 `solid()` 插件；决策数据保留在本节。
- 局限：合成压测、未测单行高频更新 / 首屏挂载 / 真实 TorrentTable（带分页）。结论方向足以否决全量迁移，但不代表所有场景。

### 验证结果（本轮开工）

- 前端：`vue-tsc --noEmit` 无错误；`vitest run` **111 文件 / 520 测试全绿**；`vite build` 成功（40.85s）。
- 后端：`go test ./internal/core/event ./internal/ws ./internal/site -short` 全绿；`go vet` 通过。
- 门禁：`ep-gate` PASS（20 个存量 views 无新增引入 EP）；`perf-gate` 已转阻断式（无采集结果时 SKIP），浏览器指标待应用实例运行后采集。
- 生成器：`307` 定义 / `438` host → `zz_defs_index.go`（`zz_` 前缀已被文件行数门禁排除）。
- 本轮（阶段 1 深挖）：`vite build` 成功（25.34s）、`vue-tsc` 无错误、**111 文件 / 520 测试全绿**、门禁 4 PASS / 0 不达标、Playwright 冒烟登录页正常渲染。
- 本轮（A 项补采）：验证后端可零配置本地起（`SAFE_MODE=1 DEMO_MODE=1 DATA_DIR=<tmp>` + `go run ./cmd/ptpatronus`，`/healthz`=200）；交付 `scripts/perf-auth-smoke.mjs` 演示登录进 `/dashboard` 采登录态指标（long_tasks=2 / 帧 P95 16.7ms / 稳态堆 10.7MB）；LCP 在 headless 无候选仍 MISSING，WS/P99/INP/Lighthouse 仍需专门压测环境。
- 本轮（P99/bench）：交付 `scripts/perf-backend-bench.mjs`（登录态对 11 个 dashboard API 压测，dashboard P99=15.6ms）与 `BenchmarkBroadcastFlush`（200 连接×10 消息合并广播，WS 广播 P99≈1.0ms）；门禁从 4 PASS → **6 PASS / 0 不达标**（新增 dashboard_p99、ws_broadcast_p99），基线固化 6 项。
- 本轮（INP）：交付 `scripts/perf-inp-probe.mjs`；**实测 INP P75≈40ms**（已纳入门禁，门禁达 7 PASS）。关键教训：后端 `go:embed` 用 `backend/internal/web/dist`，须先 `make embed-preview` 同步，否则测的是旧前端（无 glass 降级，glassReduced 恒 false、交互慢约 4 倍，会误判 208ms 超标）。blur 开/关对照 160ms vs 40ms，验证降级价值。
- 本轮（Lighthouse）：尝试用 lighthouse 13 测登录页性能分，Playwright chromium 与 Edge 均 `NO_FCP`（环境缺标准 Chrome、SPA 节流加载未出 FCP），**保持 MISSING**，改在 CI/本机 Chrome 跑（方法见 perf-baseline.md）。门禁稳在 7 PASS / 0 不达标 / 3 MISSING（lcp / long_tasks / lighthouse）。

修复记录（验证中发现并已解决）：`<script setup>` 不允许 `export`（纯函数移到 `virtualListRange.ts`）、`computeRange` 滚动越界导致 `start > end`、统一调度器在 jsdom 下误读 `document.hidden` 停掉全部轮询、`ep` 门禁脚本标识符缺空格、TorrentDrawer 残留 `statsTimer` 引用。

### 性能门禁闭环（本轮）

- `scripts/perf_baseline.json` 的 `baseline` 已固化（4 项：首屏 154.3 / 主 chunk 65 / 帧时间 P95 16.8 / 稳态堆 3.8MB），`baselineSource` 记录采集时间。
- `collect-perf.mjs` 新增 `--static-only`：跳过浏览器，只扫 dist 入口 chunk，CI 无需起服务或装 Playwright 浏览器。
- `collect-perf.mjs` 补 LCP 的 `getEntriesByType` 兜底 + CDP `Performance.getMetrics` 堆内存采集；headless 登录页确实无 LCP 候选，保持 `MISSING` 不硬凑数字。
- `check_perf.mjs --init` 只固化 thresholds 定义的指标，参考值（`cold_long_tasks` / `total_js_gzip_kb`）不进棘轮。
- `.github/workflows/ci.yml`：`frontend` job 在 `npm run build` 后执行 `Perf budget (blocking)`（静态采集 + 阻断比对）；`perf-gate` job 改名 `Performance Ratchet Gate`，`check_perf.mjs` 与 `check-no-new-ep.mjs` 均去掉 `--report-only` 转阻断。
- 本地验证：静态-only 流程门禁 `exit 0`；构造 `first_js_gzip_kb=999` 的回归样本门禁 `exit 1`（阻断确实生效）；`check-no-new-ep.mjs` 阻断模式 PASS。
- 仍需真实环境的指标：LCP / INP / Lighthouse / WS 压测长任务 / 后端 P99，当前标 `MISSING`，不算门禁失败；采集方式见 [性能基线](perf-baseline.md)。稳态堆内存已改为 CDP 自动采集并纳入基线。

### 可回退开关

- `PTP_WS_BATCH=0` → WS 批量广播退化为逐条 `Broadcast`。
- `PTP_SITE_FASTPATH=0` → `MatchByURL` 退回全量遍历。
- `perf-glass-full=1` / `setGlassBlur(true)` → 恢复模糊（`perf-glass-flat=1` 或低端机强制降级优先）。
- CI 的 `perf-gate` 与 `No new Element Plus` 已转**阻断式**；需要临时放宽时改回 `--report-only`。