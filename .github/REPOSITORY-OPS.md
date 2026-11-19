# 仓库社区运营配置基线

> 维护者备忘：仓库设置里已开启/推荐开启的社区功能。改这里 = 改运营口径。

## 已开启（2026-11-19）

- **Discussions**：`has_discussions=true`。定位分工——
  - **Q&A**：部署/配置疑问（issue 只收 bug 与适配问题，问如何配走 Discussions）
  - **Ideas**：功能讨论（成熟了再转 issue 立项）
  - **Show and tell**：用户晒部署/工作流分享
  - Announcements：发版公告（维护者手动发，置顶最新版；首个帖子 #38 v1.03.32。
    GraphQL 无置顶 mutation，**置顶需网页端手动**；有自动发帖需求可挂 publish-release 后续步骤）
- **Issue 模板**三份：Bug 报告 / 功能建议 / **站点适配问题**（site-adaptation 标签，
  带「引擎类型/症状/自检确认」结构化字段——站点定义是全实例共享热更新的，
  该类 issue 同时是 304 站适配的维护工单源）。

## 运营节奏（约定）

- 发版时：Release notes 由 publish-release workflow 从 CHANGELOG.md 自动提取段落（整改④）；
  同步在 Discussions → Announcements 发一帖（标题 ` vX.Y.Z 发布`，正文贴同一段）。
- 站点适配 issue：打 `site-adaptation` 标签；修复后跨仓库同步到
  ptpatronus-sites 定义仓库发版，回链 issue 关闭。

## Star History

README 尾部已挂 star-history.com 动态图（整改⑤），有星之后图会自然生长。
