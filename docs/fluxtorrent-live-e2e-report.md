# FluxTorrent 真实联调报告

日期：2026-11-19
对象：本机 Docker FluxTorrent 栈（flux-api :8080 / flux-web / tracker / postgres / redis，镜像 0.5.19-dev.13）
方式：使用本地测试 Token 真实调用开放 API，跑 `TestFluxTorrentLiveE2E` 端到端。

---

## 一、联调结果

真实 e2e 从失败到通过，覆盖：搜索种子（含大小/免费/DVF）、用户信息+等级、H&R/做种/消息能力端点、下载直链构造。

| 环节 | 结果 | 实测数据 |
|---|---|---|
| 搜索 | ✅ | `FluxTorrent 对接自测种子 (demo)` 96 B，dvf=1 |
| 用户信息 | ✅ | id=1 / root / 站长 / up=2.2G / down=5.7G / bonus=14155 |
| 能力端点 | ✅ | hr=0 / seeding=0 / messages=50 |
| 下载直链 | ✅ | `.../compat/nexusphp/download.php?id=40079`（无 passkey 模板链，由 chain 通用补全） |

## 二、联调发现的真问题：三边契约漂移

**现象**：e2e 首跑失败于 `passkey len = 0, want 32`。

**根因**：FluxTorrent 站点源码（`apps/api/src/compat_http/ptpp.rs`）已明确改为「ptppUserInfo 不返回 passkey；下载请先签临时下载凭证」（Utoipa 注释），并在 2026-11-18 新增 `seedingPoints` 做种积分字段；但 PTPatronus 适配器的注释与 e2e 仍按旧契约强制要求 passkey，且未解析 seedingPoints。这正是项目备忘里「改站点行为要同时看 PTPatronus / FluxTorrent / PTD 三边」的典型漂移。

**修复**（[fluxtorrent.go](../backend/internal/site/schemas/fluxtorrent.go) + [flux_e2e_test.go](../backend/internal/site/schemas/flux_e2e_test.go)）：
- `fluxPTPPData` 增加 `SeedingPoints` 字段并映射到 `UserInfo.SeedingBonus`（对齐 PT-Plugin-Plus 用户卡「积分」列）。
- 修正过时注释；passkey 字段保留仅为兼容旧站点。
- e2e 改为验证新契约：不再要求 passkey，改为断言下载走无凭证模板链 + seedingBonus 非负。

## 三、另一个发现：运行镜像落后源码

`seedingPoints` 实测为 undefined。核对发现：flux-api 镜像建于 **2026-11-17**，而 `seedingPoints` 的源码提交 `d05cc905` 在 **2026-11-18**——运行镜像比源码旧一天，新字段还没构建进镜像。

结论：这不是 PTPatronus 的 bug；schema 的前瞻解析是正确的，待站点重新构建镜像后即可拿到非零做种积分。联调同时验证了「本地跑的是 ghcr 镜像而非源码构建」这一环境事实（项目备忘亦有记录）。

## 四、联调资产

- 测试 Token：`fxo_ptpatronus_e2e_local_test_20261119`（root/uid=1）与 `fxo_ptpatronus_e2e_seeduser_20261119`（uid=8444），仅本地 DB，SHA3-256 存 api_tokens。
- e2e 入口：`FLUX_E2E_TOKEN=<token> FLUX_E2E_BASE=http://127.0.0.1:8080 go test ./internal/site/schemas -run TestFluxTorrentLiveE2E -v`。
- 未设置 FLUX_E2E_TOKEN 时该测试自动 skip，不影响 CI。

## 五、结论

真实联调跑通了 FluxTorrent 适配器的全链路，并抓出单测/mock 无法发现的跨仓库契约漂移，已修复。剩余待办：站点侧重构镜像后复验 seedingPoints 非零（需 FluxTorrent 仓库操作，不在本仓范围）。
