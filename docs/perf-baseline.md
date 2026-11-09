# 性能基线与门禁

配套文档：[性能与流畅重构方案](perf-refactor-plan.md)。本文件定义「流畅」的可度量口径。

## 棘轮语义

照抄 `scripts/check_file_length.sh` 的模式：

- 超过 `scripts/perf_baseline.json` 的 `thresholds` → **FAIL**（硬阈值不达标）。
- 比 `baseline` 差（含 10% CI 波动容差）→ **REGRESS**（只许变好不许变坏）。
- `baseline: null` 时只做阈值检查；基线已用 `make perf-baseline-init` 固化（当前 3 项）。
- CI 两处均为**阻断式**：`frontend` job 在 `npm run build` 后用 `--static-only` 扫描 dist 并比对；`perf-gate` job 比对棘轮基线（无采集结果时 SKIP，不算失败）。临时放宽用 `--report-only`。

## 五个固定场景

| id | 场景 | 主要指标 |
|---|---|---|
| `cold-start` | 清空缓存访问 `/` | LCP、首屏 JS gzip、长任务 |
| `dashboard-range` | 仪表盘 7/30/90 天切换 | INP P75 |
| `torrent-scroll` | 1000 条种子列表匀速滚动 | 帧时间 P95 |
| `ws-burst` | WS 1000 msg/s 压测 | 主线程长任务数、后端广播 P99 |
| `terminal-camera` | 摄像头/终端面板打开 | 稳态内存、帧时间 |

## 指标口径

| 指标 | 目标 | 采集方式 |
|---|---|---|
| `lcp_ms` | < 2500 | PerformanceObserver + `getEntriesByType` 兜底；headless 登录页无候选时标 MISSING |
| `inp_p75_ms` | < 200 | Playwright 交互 + PerformanceObserver `event` |
| `frame_p95_ms` | < 20 | rAF 帧间隔采样（3s / 180 帧）P95 |
| `long_tasks` | 0 | PerformanceObserver `longtask`（>50ms） |
| `first_js_gzip_kb` / `main_chunk_gzip_kb` | < 300 / < 150 | 扫描 `frontend/dist/assets/*.js` 逐文件 gzip |
| `lighthouse` | ≥ 90 | Lighthouse CI（移动端节流）；本环境无可跑浏览器，见下方小节 |
| `ws_broadcast_p99_ms` | < 5 | Go benchmark `./internal/ws` 的 `BenchmarkBroadcastFlush`（200 连接×10 消息合并广播 P99） |
| `dashboard_p99_ms` | < 100 | `node scripts/perf-backend-bench.mjs`（登录态对 11 个 dashboard API 各打 60 次取 P99） |
| `steady_memory_mb` | < 150 | CDP `Performance.getMetrics` 的 `JSHeapUsedSize`（采集器自动，冷启动口径） |

未采集的指标在门禁里标 `MISSING`，不算失败——避免 CI 因环境缺项长期飘红。

## 常用命令

```bash
make perf                 # 采集（需前端已 build；可选起本地服务测浏览器指标）
make perf-check           # 门禁（阻断模式）
make perf-baseline-init   # 把本次结果固化为新基线
node scripts/collect-perf.mjs --static-only             # 只测静态体积（CI 用，无需起服务）
node scripts/check_perf.mjs --results perf-results.json   # 阻断式门禁
node scripts/perf-backend-bench.mjs --base http://127.0.0.1:8088   # 登录态 dashboard API P99（需后端）
cd backend && go test -bench BenchmarkBroadcastFlush -benchtime 20000x -run '^$' ./internal/ws   # WS 广播 P99
```

后端指标（WS 广播 P99、`/dashboard` P99）用 `go test -bench` / pprof 采集后手动写入 `perf-results.json` 的 `metrics`，再跑门禁。

## 本地登录态采集（补齐 dashboard 指标）

未登录首屏只能测登录页；dashboard 的真实渲染成本需登录后测。本地零配置起后端 + 演示登录：

```bash
# 后端（SQLite、临时数据目录；SAFE_MODE 跳过插件/调度/监控，DEMO_MODE 开演示登录）
cd backend && SAFE_MODE=1 DEMO_MODE=1 DATA_DIR=$(mktemp -d) HTTP_ADDR=:8088 go run ./cmd/ptpatronus
# 另开终端：演示登录进 /dashboard，采长任务/帧时间/稳态堆
node scripts/perf-auth-smoke.mjs --base http://127.0.0.1:8088
```

- 脚本走 `POST /api/v1/auth/demo-login` → `/dashboard` 采样 150 帧，输出 `perf-auth.json`。
- 采到：`long_tasks`（dashboard 挂载期长任务，**仅观测**，不是门禁的 WS 压测零长任务口径）、`frame_p95_ms`、`heap_mb`。
- `lcp_ms` 在 headless 下常为 0（无 LCP 候选），属正常，需真实浏览器 + 交互才有意义。
- 仍需专门压测环境：`lighthouse`（需 Lighthouse CI）。

### INP 交互回放（登录态，已纳入门禁）

```bash
node scripts/perf-inp-probe.mjs --base http://127.0.0.1:8088
```

登录后有序点击（导航折叠、通知抽屉、搜索），采 event timing P75。**实测 P75≈40ms**（含通知抽屉等重交互），远优于 200ms。

对照实验（同机同一交互）：backdrop-filter 开 → P75≈160ms；glass 降级关 → P75≈40ms，**降级带来约 4 倍交互提速**，验证 usePerfMode 默认降级的价值。

> ⚠️ 后端 `go:embed` 的是 `backend/internal/web/dist`（不是 `frontend/dist`）。本地测前须先 `make embed-preview` 同步，否则后端 serve 旧前端构建（无 glass 降级 → glassReduced 恒 false、交互慢约 4 倍），会误判 INP 超标。

### Lighthouse（本环境跑不通，需 CI / 本机 Chrome）

本机 lighthouse 13 对 `vite preview` 的 SPA 登录页，在 Playwright 定制 chromium 与系统 Edge 下均报 `NO_FCP`（headless 节流加载下首屏未出 FCP）。属**运行环境缺标准 Chrome**，非代码问题。请在装 Chrome 的 CI 或本机执行：

```bash
npx lighthouse http://127.0.0.1:4173/ --only-categories=performance --output=json
```

移动端节流，把 `categories.performance.score*100` 写入 `perf-results.json` 的 `metrics.lighthouse` 再跑门禁（阈值 90）。