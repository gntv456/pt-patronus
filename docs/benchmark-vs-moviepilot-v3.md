# 对标 MoviePilot v3 的产品差距分析

日期：2026-11-19
参照：jxxghp/MoviePilot 分支 v3（v3.2.8），一手资料来自其 GitHub 仓库 `README.md`、`docs/v2-to-v3-overview.md`、`docs/mcp-api.md`；本地另有 v2.14.1 源码与插件仓库（v1 142 / v2 91 插件）交叉印证。
说明：MP 是「影视自动化」赛道，PTPatronus 是「PT 站点管理+插件生态」赛道，定位不同。本报告只提取**机制层面**可借鉴点，不盲目对标功能数量。

---

## 一、我们已具备、无需改（避免重复造轮子）

| MP v3 机制 | 我们的对应 | 结论 |
|---|---|---|
| 网页首启创建管理员、不写初始密码到环境变量 | docker-compose 模板 `CHANGE_ME_BEFORE_DEPLOY` 强制改密 + 首启向导 | 已覆盖 |
| 可靠事件（重启不漏通知） | notice 状态机 LastSince/Recover + 外发重试 | 已覆盖（前几轮整改） |
| 数据保留期设置 | 签到/站点快照/插件日志/通知四类 TTL 清理已接线 cron | 已覆盖 |
| 媒体来源统一 `media_source+id` | `MediaSourceProvider` + media_source_id 错误码 | 已有雏形 |
| MCP 端点 | 我们有 Agent/MCP 相关能力（见 agent 包） | 已有 |
| Redis/PostgreSQL 可选 | SQLite(纯Go) + PostgreSQL 双驱动 | 已覆盖 |

## 二、对标发现的真实差距

### D1（已整改）迁移前自动备份 —— 高

MP v3：「数据库结构迁移前可以自动创建备份」「备份管理、恢复校验」。我们：`db.Open` 直接 AutoMigrate + RunMigrations，**迁移前无任何物理快照**；而项目反复强调 `./data` 目录删除即丢失全部站点 Cookie/凭据。这是数据资产安全的实质缺口。

**整改**：新增 `backend/internal/db/migration_snapshot.go`，在 AutoMigrate 前用 SQLite `VACUUM INTO` 生成事务一致性物理快照到 `data/backups/pre-migration/`，保留最近 5 份；空库跳过、快照失败仅告警不阻断启动。3 项单测覆盖（空库跳过 / 快照含迁移前数据且不被后续变更污染 / trim 保留 5 份）。

### D2（建议，未做）插件虚拟分身 / 来源绑定 —— 中

MP v3：同一插件源码可创建多个独立实例（配置/数据/事件隔离），并记录插件来自哪个市场、支持换源。我们：已有 `PluginInstance` 模型（manifest 中有），但能力边界与前端入口需实测确认是否完整。建议作为独立迭代验证，不在本轮盲改。

### D3（建议，未做）Agent 敏感设置二次确认 —— 中

MP v3：「读取敏感设置时需要宿主二次确认，避免模型直接获取密钥」。我们的 Agent/LLM 工具链若暴露配置读取能力，应同样加确认闸门。建议下轮审计 agent 工具权限时专项处理。

### D4（建议，未做）分类策略 revision 版本化与影响分析 —— 低

MP v3：自动分类按 revision 版本化、发布前校验+历史样本影响分析、并发冲突提示。我们的分类规则相对静态，规模小，优先级低。

## 三、结论

对标 v3 后，唯一「别人有而我们完全没有、且直接关系数据资产安全」的缺口是迁移前备份，本轮已整改并测试。其余差距（插件分身、Agent 二次确认、分类版本化）属于增强项，已记录，建议按独立迭代推进，避免一次性大改引入回归。
