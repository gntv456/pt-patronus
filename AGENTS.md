# AGENTS.md — HxPT / FluxTorrent 远端排查记录（本地工作区 PTPatronus 索引副本）

> 注意：本文件是 **本地工作区 `D:\PTPatronus`（PTPatronus 项目）** 下的一份索引/记录副本。
> 被排查的真实代码在**远端服务器 159.195.247.176 的 `/opt/fluxtorrent/src-build`**（HxPT 站点 www.hxpt.org，FluxTorrent 0.5.16）。
> 服务器上同名 `AGENTS.md` 为权威版本，本文件与之内容一致。
> 记录时间：2026-11-09

# 记录正文

本文件记录用户提出的 3 个问题：**根因、涉及文件、修复方案、验证方法**。后续 agent/人接手请先读本文件。
（完整正文见服务器 `/opt/fluxtorrent/src-build/AGENTS.md`，下述与其一致。）

---

## 0. 系统速览

- 架构：Rust 后端 `apps/api`（actix，:8080）+ Next.js 前端 `apps/web`（:3000）+ tracker(:7070) + worker，Postgres(:5432)+Redis(:6379)，docker compose。
- 部署目录：`/opt/fluxtorrent/docker/`（`docker-compose.yml` + `.env`，`FLUX_VERSION=0.5.16`）。
- 源码目录：`/opt/fluxtorrent/src-build/`。
- 鉴权：HttpOnly cookie `flux_token`（真凭证）+ `flux.session`（标记）；JWT HS256，密钥为 `.env` 的 `JWT_SECRET`；Claims=`{sub,class_id,exp,iat}`。
- 服务端 RSC 调 api：`api-client.ts` 从 `next/headers cookies()` 取 `flux_token` 转 `Authorization: Bearer`，直连 `API_SERVER_URL=http://api:8080`。
- **改代码后须重建镜像并 `docker compose up -d` 才生效**（见 §4）。

---

## 1. Bug ①：缺的那天不能点击补签

### 现象
签到日历里漏签日（`is-missed`）格子是纯展示 `<span>`，点不动；补签唯一入口是"补签卡"弹窗，只能从**过去 7 天**固定列表选，且不区分已签/漏签。

### 根因
- `apps/web/components/home-attendance.tsx`：日历格子渲染成 `<span className={cls}>`，**无 onClick**。
- `apps/web/components/home-resub-button.tsx`：`days` = 过去 7 天固定列表，与日历漏签日无联动；点已签日会吃"该日已有签到记录"报错。
- 后端 `apps/api/src/gaps_http/misc.rs::resub_use`：`days_ago` 限制 `(1..=7)`（只许补过去 7 天内）；而设计稿写的是"近 30 天内可补"，三者口径不一致。

### 修复方案
1. 日历漏签格改可点：`!d.done && !isFuture` 时用 `<button onClick={() => onPickMakeup(d.date)}>`，复用 `doResub(date)`。
2. 补签弹窗列表改为"过去 N 天内且未签到"的日期。
3. 统一可补范围：后端 `(1..=7)` 与设计/文案统一（如 30 天），二者同改。

### 涉及文件
- `apps/web/components/home-attendance.tsx`
- `apps/web/components/home-resub-button.tsx`
- `apps/api/src/gaps_http/misc.rs`（`resub_use`）

---

## 2. Bug ②：补签不给魔力（显示 +0）、补签后连签不续

### 现象
截图：补签那天（6 号）`+0`；4/5 号 `+1000`；补签后连签没接着涨、后续奖励没按连签继续。

### 根因（后端写死 reward=0、streak=0）
`apps/api/src/gaps_http/misc.rs::resub_use`：
```sql
INSERT INTO attendance (user_id, date, streak, reward, makeup)
VALUES ($1, $2, 0, 0, TRUE) ON CONFLICT (user_id, date) DO NOTHING
```
注释："补签出勤行：streak 不续（补的是漏签日）、reward=0（火花在正常签到发放）"。
即按 §9.3 定案："补签只补考勤痕迹、不续连签、不发魔力"。`shown_streak()`（`economy_checkin.rs`）会**排除 makeup 行**算连签，所以补签不续签——正是"补签后连签没生效/奖励没继续"的原因。

### 结论：这是**产品策略**，不是 bug
"补签也发魔力+续连签"是产品需求变更，需产品确认后再改。

### 若要改成"补签发魔力 + 续连签"
1. 计算补签日应发奖励（通常发基础连签值 `CheckInParams.streak`，不叠加里程碑）。
2. INSERT 写真实 `streak`（补上后应处连签值）与 `reward`，并在事务内给余额加等值魔力。
3. `shown_streak`/`attendance_json`/签到主流程对 makeup 行口径**一致**调整，同步改测试 `shown_streak_decays_on_gap`。
4. 前端 `d.reward` 自然显示真实值，无需改渲染。
5. 历史已补签的 +0 记录需单独一次性补发脚本。

### 涉及文件
- `apps/api/src/gaps_http/misc.rs`
- `apps/api/src/economy_checkin.rs`
- `apps/api/src/http/home_attendance.rs`
- 测试：`economy_checkin.rs` 内 `#[cfg(test)]`

---

## 3. Bug ③：首页"滚动推荐"没有显示海报

### 现象
首页顶部"滚动推荐海报带"（`PosterRibbon`）与底部"最新资源"海报墙（`LatestPosters`）都不出真实海报；顶部带渲染的是生成式占位（彩色渐变 + 🎬 的 SVG）。

### 根因（两个独立问题，实测确认）

#### ③-A（主因）：后台 `home_layout` 自定义排版没有 `latest` → 首页根本不渲染海报带
`apps/web/app/(main)/page.tsx`：
```ts
let showLatest = true;
if (homeData) {
  const layout = parseHomeLayout(homeData.home_layout, homeData.home_sections ?? []);
  if ((homeData.home_sections?.length ?? 0) > 0 || homeData.home_layout?.trim())
    showLatest = layout.some((x) => x.key === "latest");
}
if (showLatest && showcaseOn) { latest = await paged("/api/v1/torrents", { limit: 12 }); }
```
实测该站 `/api/v1/home`：`home_sections` 含 `{"key":"latest"}`，但 `home_layout`=`[news, attendance, shoutbox, funbox, site_data, lucky_draw, links]`（**无 latest**）。`parseHomeLayout` 只保留显式键、不自动补 latest → `showLatest=false` → 不拉 `/api/v1/torrents` → `posterItems=[]` → 全占位 + 最新资源空态。

**证据**：直连 `/api/v1/torrents?limit=12`（Bearer）返回 12 条真实海报（doubaninfo.com）；SSR 首页 HTML 中 `href="/torrent/N"`=0、`w-[150px]`=0、`w-[88px]`=20、`data:image/svg`=20。

**修复**：
- 运营侧（最快）：后台「首页排版」把 `latest` 加回并保存。
- 代码侧：`parseHomeLayout` 对 `in_default=true` 但缺失的键补回（或页面不再因排版漏项整体隐藏海报带）；需产品确认隐藏 latest 是否合法。

#### ③-B（次因）：海报代理路由未注册 → doubanio/tmdb 海报 404
- `apps/web/lib/poster-src.ts` 把 doubanio/tmdb 图改写为 `{NEXT_PUBLIC_API_URL}/api/v1/works/poster-proxy?url=...`。但 `NEXT_PUBLIC_API_URL=https://www.hxpt.org/api` 已含 `/api`，再拼 `/api/v1` → **双 /api**。
- 后端 `apps/api/src/torrent_http/works_poster_proxy.rs::poster_proxy` **已定义导出，但全仓无 `.service(poster_proxy)` 注册** → `/api/v1/works/poster-proxy` 实测 **404**；豆瓣直连无 Referer **418**。其它图床（doubaninfo/pixhost/ipic）实测 200 可显示，故只有 doubanio/tmdb 图挂。

**修复**：
1. 在 `apps/api/src/http/mod.rs`（或 gaps scope）补 `.service(crate::torrent_http::poster_proxy)`。
2. `poster-src.ts` 修基址拼接：`NEXT_PUBLIC_API_URL` 已含 `/api` 时不再拼 `/api`。
3. 验证：`curl 'https://www.hxpt.org/api/v1/works/poster-proxy?url=<豆瓣图>'` 应 200 `image/*`。

### 涉及文件
- `apps/web/app/(main)/page.tsx`
- `apps/web/components/home-layout-parse.ts`
- `apps/web/lib/poster-src.ts`
- `apps/api/src/torrent_http/works_poster_proxy.rs`（无需改）
- `apps/api/src/http/mod.rs`（补路由挂载）

---

## 4. 改完如何生效（重建 + 重启）

```bash
cd /opt/fluxtorrent/docker && docker compose up -d --build web api
```
- compose 里 web/api 同时有 `image`+`build`，`--build` 优先本地构建。
- 迁移只由 api 启动执行；本轮无需迁移。
- 前端 `NEXT_PUBLIC_*` 是构建期烧入，改 `.env` 的 `NEXT_PUBLIC_API_URL` 必须重建 web 镜像，光重启无效。

---

## 5. 复验清单

用 HS256 现场签测试 JWT（`JWT_SECRET` 取自 `docker/.env`）：payload=`{sub:8,class_id:98,iat:now,exp:now+3600}`。
- ③：`curl -s -H "Cookie: flux_token=<jwt>; flux.session=1" http://127.0.0.1:3000/ | grep -c 'href="/torrent/'` 应 >0。
- ③-B：`/api/v1/works/poster-proxy?url=<豆瓣图>` 应 200（修前 404）。
- ②：`SELECT date,streak,reward,makeup FROM attendance WHERE user_id=<id> ORDER BY date DESC LIMIT 3;` 看 reward 是否仍 0。

---

## 6. 关键文件索引

| 功能 | 文件 |
|------|------|
| 首页入口（showLatest/posterItems） | `apps/web/app/(main)/page.tsx` |
| 海报带组件 | `apps/web/components/latest-posters.tsx` |
| 海报渲染/代理改写 | `apps/web/components/poster-image.tsx`、`apps/web/lib/poster-src.ts` |
| 海报代理后端 | `apps/api/src/torrent_http/works_poster_proxy.rs` |
| 签到日历（前端） | `apps/web/components/home-attendance.tsx` |
| 补签弹层（前端） | `apps/web/components/home-resub-button.tsx` |
| 补签接口/发奖 | `apps/api/src/gaps_http/misc.rs` |
| 签到经济/连签口径 | `apps/api/src/economy_checkin.rs` |
| 首页签到个人段 | `apps/api/src/http/home_attendance.rs` |
| 排版解析 | `apps/web/components/home-layout-parse.ts` |

---

## 7. 修复实施记录（2026-11-09 已完成并上线）

本次已按 §1–§3 方案全部实施，重建镜像并切换生产容器（`fluxtorrent-api:hotfix1109` / `fluxtorrent-web:hotfix1109`，tag 回 `ghcr.io/gntv456/fluxtorrent/{api,web}:0.5.16`）。

### 已改文件（均带 `.bak-20261109` 备份）
- ③-A（配置热修，无需发版）：`site_settings.home_layout` 追加 `{"key":"latest","span":3}`；清 Redis `cache:home:shared:v3` 缓存。**注意 UPDATE 时双引号被 shell 吃掉的坑**——最终用 `docker cp` + `pg_read_file` 方式写入正确 JSON。
- ③-B：`apps/web/lib/poster-src.ts` 的 `apiBase()` 去末尾 `/api`（防双 /api）。实测现网 `NEXT_PUBLIC_API_URL` 构建期为空（客户端走同源相对），本就无双 api，此改为防御性。海报代理路由 `/api/v1/works/poster-proxy` **本已注册**（http/mod.rs:149），真实豆瓣图实测 200。
- ①：`home-attendance.tsx` 日历漏签格改可点 `<button is-makeup-clickable>`，点击直接补签该日；`home-resub-button.tsx` 抽出 `useResub` hook、弹窗日期改"近 30 天未签到"；`gaps_http/misc.rs` 的 `days_ago` 由 `(1..=7)` 改 `(1..=30)`；`layout.css` 加 `.is-makeup-clickable` 样式；i18n 文案「7 天」→「30 天」。
- ②：`gaps_http/misc.rs::resub_use` 补签行改**写真实 streak + 基础连签魔力**（`checkin_reward(streak,false,params).base`），同事务 `earn_spark_tx(..., "attendance_makeup", ...)` 入账；返回体加 `reward/streak`。`economy_http/mod.rs` 的 `mod spend` → `pub(crate) mod spend` 以供跨模块调 `earn_spark_tx`。`checkin.rs`（签到主流程 + 状态接口）、`home_attendance.rs` 三处连签基线由「排除 makeup 行」改为「取最后一条记录（含补签）」。

### 实测验证（用 HS256 现场签 user 8 的 JWT）
- ③：首页 `href=/torrent/N`=24、`w-[150px]`=24、`data:svg` 占位=0；海报代理真实豆瓣图 200 image/jpeg。
- ①：11-07 漏签格渲染为 `<button class="...is-missed is-makeup-clickable" title="…点击补签（消耗 1 张补签卡）">`。
- ②：补签 2026-11-07 返回 `{reward:8, streak:24}`，余额 1015651→1015659（+8），attendance 行 `streak=24,reward=8,makeup=t`，补签卡 3→2。
- 测试数据已全部回滚清理（删除临时发放卡/补签记录/魔力入账，余额复原）。

### 已知限制（重要）
补签**只修正被补那一天及其后续新签到**的连签，**不会回改历史已断签的记录**。例：user 8 在 11-06 后断签，11-08 已落 streak=1、11-09 streak=2；补签 11-07 后 11-07 得 streak=24，但 11-08/11-09 仍是 1/2（历史值不变）。若产品要求「补签后整条连签链重新接上」，需额外一次性的历史重算脚本（遍历补签日之后所有行重算 streak），本轮未做。

### 重建命令（本次实际用法）
```bash
cd /opt/fluxtorrent/src-build
docker build -f docker/api.Dockerfile -t fluxtorrent-api:hotfix1109 .   # ~20min(LTO)
docker build -f docker/web.Dockerfile -t fluxtorrent-web:hotfix1109 .
docker tag fluxtorrent-api:hotfix1109 ghcr.io/gntv456/fluxtorrent/api:0.5.16
docker tag fluxtorrent-web:hotfix1109 ghcr.io/gntv456/fluxtorrent/web:0.5.16
cd /opt/fluxtorrent/docker
docker compose up -d --no-build --force-recreate api   # 先 api(迁移) 后 web
docker compose up -d --no-build --force-recreate web
```
注意：`docker compose build`（bake 模式）会因 `context: ..` + `dockerfile: docker/api.Dockerfile` 解析错路径报「api.Dockerfile not found」——改用上面的显式 `docker build -f` 再 `up -d --no-build` 绕过。

---

## 8. 历史连签重算（2026-11-09 已执行）

因 §7 的补签修复只影响「新补签」，历史已断签记录不会自动修正。按用户要求执行了**全量历史重算**。

### 规则
对所有存在 makeup 行的用户（998 人），按 date 升序重排 streak：
streak(d) = 1 if (d-1) 无记录 else streak(d-1)+1（补签行也参与，与 §7 新逻辑一致）。
**只改 streak，不动 reward / makeup**。

### 执行与结果
- 备份：CREATE TABLE attendance_bak_20261109 AS SELECT * FROM attendance;（654,112 行）
- UPDATE：156,995 行 streak 被更正（覆盖 997 个用户）。
- 验证：重跑 dry-run = 0 行变化（幂等）；无 streak<1 异常；总行数 654,112 不变；清 cache:home:shared:v3。
- 脚本位置：服务器 /tmp/restreak.sql、/tmp/do_restreak.sql。

### 回滚方法（如需）
UPDATE attendance a SET streak = b.streak FROM attendance_bak_20261109 b
WHERE a.user_id=b.user_id AND a.date=b.date AND a.streak<>b.streak;
（或整表：TRUNCATE attendance; INSERT INTO attendance SELECT * FROM attendance_bak_20261109;）

### 注意
- 重算会让长期每日都签到（含补签）的用户连签数字明显变大（真实连续天数）。
- attendance_bak_20261109 备份表建议保留一段时间后再删。


