# PTPatronus 产品复审（第二轮）· 开放生态与工程治理

日期：2026-11-19  
范围：第一轮 PM 整改已完成并闭环；本轮聚焦上一轮未触及的「开放生态基础设施」与「工程治理」领域。  
原则：每条不足都附可证伪的证据与对应的整改动作，不提无法落地的空建议。

---

## 一、新发现（按严重程度）

### G1 · 缺安全披露政策（SECURITY.md）—— 高

**证据**：仓库根无 `SECURITY.md`。项目是 AGPL 开源、含 19 个通知通道凭据、站点 Cookie/API Key 加密存储、浏览器扩展向白名单站点注入凭据，且已有公开 issue 模板。用户或研究员发现漏洞时，没有规定的私密披露渠道，只能发公开 issue——这会把未修复漏洞（如插件 SSRF、路径穿越）暴露给所有人。

**整改**：新增 SECURITY.md，明确 report 邮箱/GitHub private vulnerability reporting、响应时限、支持版本范围、已知边界（单用户自托管、插件为独立进程）。

### G2 · 缺贡献指南（CONTRIBUTING.md）—— 高

**证据**：项目有 304 个站点定义、34 个市场插件、SDK 与 CI 门禁（400 行/80 列棘轮、errcode 唯一性、三端测试），但没有 CONTRIBUTING.md 告诉外部贡献者：如何本地起三端、新增站点定义流程、插件如何打包签名、PR 需要跑哪些检查。这直接限制生态贡献。

**整改**：新增 CONTRIBUTING.md，把散落在 README/07-开发指南、15-plugin-development、发布指南里的关键流程收敛成一页入口，并明确 PR 前必跑的检查命令。

### G3 · 版本号四处硬编码，靠人工同步 —— 中

**证据**：`backend/internal/version/version.go`(1.03.31)、`frontend/package.json`(1.03.31)、`mobile/pubspec.yaml`(1.03.31+44)、`update-manifest.json`(latestVersion) 四处独立维护；上一轮已发现 version.go 注释写着旧版本 1.03.29。文档声称 version.go 是单一真源，但实际其它三处仍手工同步，漂移只是时间问题。

**整改**：新增 `scripts/check-version-sync.mjs`，CI 在 plugin-sdk job 校验四处版本一致（pubspec 用 `+build` 拆分后比较），发布脚本前置调用。

### G4 · 26 篇 README 无导航索引 —— 中

**证据**：`README/` 目录有 26 篇 md，README.md 只链了「快速开始」等 4 篇；用户要找「部署到 NAS」「i18n 评估」「插件开发」得靠文件列表猜。

**整改**：在 README/00-项目总览.md 补一个按人物角色分组的完整文档索引表（使用者/部署者/插件开发者/维护者）。

### G5 · 隐私边界未在用户面前显式化 —— 中

**证据**：`group-chat-zone` 插件 README 已写 Cookie 明文配置风险，但这是插件级文档；产品主文档只说「Cookie 加密存储」（服务端），没有一份统一的「隐私与数据边界」说明：什么加密存、什么明文、什么发给第三方（通知通道/AI 网关/字幕翻译/OpenAI 兼容接口）、插件能读到什么。

**整改**：新增 `README/隐私与数据边界.md`，并在 README.md 主索引加链接。

---

## 二、已在前几轮闭环、本轮不重复的领域

- 插件功能/协议/权限缺陷（P1–P12，四轮测试）
- 首启引导、Cookie 获取引导、适配器健康、告警状态机、移动端插件页
- 发版闭环（plugin_release.py + --no-bump）、脱敏诊断、ws 契约测试
- ADR-001/002/003 战略决策

---

## 三、整改验收

每项整改均落盘文件并接入 CI/校验脚本；详见对应提交。
