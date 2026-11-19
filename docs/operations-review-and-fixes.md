# PTPatronus 运营复审与整改报告

日期：2026-11-20
视角：运营团队（转化漏斗 / 反馈闭环 / 发布沟通 / 外链治理）
方法：以一手证据核对用户可见链路，能证伪的才列为缺口，逐项整改并加防回归门禁。

---

## 一、核心发现（按运营影响排序）

### O1 · P0 反馈入口 404 —— 已整改

**证据**：前端 `useGeneralSettings.ts` 的 `ISSUES_URL` / `README_URL` 硬编码为 `https://github.com/hxpt/ptpatronus/...`。实测该地址 **HTTP 404**（`gntv456/pt-patronus` 才是真实仓库，HTTP 200；remote 也是 `gntv456/pt-patronus`）。用户点「设置 → 关于 → 问题反馈 / 更新日志」会跳到空页——这是反馈收集漏斗的直接断点。移动端 `account_page.dart` 用的却是对的 `gntv456/pt-patronus`，两端不一致。

**整改**：前端两条外链改为 canonical 地址（与移动端、README、SECURITY/CONTRIBUTING 一致）。

### O2 · P1 User-Agent 自报错误仓库 —— 已整改

**证据**：`backend/internal/media/{bangumi,musicbrainz,cunguangji}.go` 的 HTTP User-Agent 自报 `https://github.com/hxpt/ptpatronus`（404）。第三方元数据源据此识别客户端身份与运营归因，报 404 地址损害可信度。

**整改**：三处 UA 改为 canonical 地址。

### O3 · P1 无外链一致性门禁 —— 已整改

**证据**：O1 的错误地址长期存在（至少从 1.03.21 起），说明没有任何机制拦截链接漂移——手动改一处、忘改另一处是必然。

**整改**：新增 `scripts/check-user-facing-links.py`，扫描前端/移动端/媒体 UA 的用户可见外链，凡指向历史错误 owner/repo 即 exit 1。已验证：注入错误链接能精确报出文件:行号并 FAIL，恢复后 PASS。接入 CI 与 pre-push 双门禁。

### O4 · 已具备、无需整改（避免重复劳动）

| 运营能力 | 现状 |
|---|---|
| 首启激活漏斗 | 三步引导 + 下一最佳行动卡（数据驱动，可关闭）已具备 |
| Cookie 获取引导 | 站点页展开式步骤 + 扩展入口已具备 |
| 帮助/黑话/反馈入口 | 关于页帮助弹窗、PT 黑话释义、反馈入口已具备（本次修好跳转） |
| 版本发布沟通 | CHANGELOG + 更新检查对话框（releaseNotes/强制更新提示）+ update-manifest 已具备 |
| 社区治理 | CONTRIBUTING / SECURITY / issue 模板（bug/feature/site_adaptation）/ Discussions 入口已具备 |
| 用户隐私信任 | 隐私与数据边界文档 + 脱敏诊断信息已具备 |

## 二、方法论说明

运营整改不等同于「多加功能」。本轮只处理有明确证据、影响真实用户转化/信任的链路问题（404 反馈入口、错误 UA），并把一次性修复转化为自动化门禁（防回归）。对已具备的能力不做重复建设。

## 三、结论

本轮修复 1 个 P0（反馈入口 404）、2 个 P1（UA 错误仓库、无外链门禁），新增 1 道 CI/pre-push 双门禁。用户从「设置 → 关于」发起反馈/查更新日志的链路现已打通，且链接漂移会被自动拦截。无遗留未落地项。
