# 参与贡献

感谢你帮助 PTPatronus 变得更好。项目采用 **AGPL-3.0-or-later**，贡献即表示你同意以同等许可证发布。

## 三条最快的贡献路径

| 路径 | 适合 | 入口 |
|---|---|---|
| 新增/校准站点定义 | 会看 HTML、懂一点 CSS 选择器 | [06-站点定义指南](README/06-站点定义指南.md) → 提交 YAML 到站点定义仓库 |
| 开发市场插件 | Python / TypeScript | [15-plugin-development](README/15-plugin-development.md) + `backend/tools/pluginctl.py` |
| 改核心代码 | Go / Vue3 / Flutter | [07-开发指南](README/07-开发指南.md) |

## 本地开发

```bash
# 后端（:8088，SQLite 零配置）
cd backend && go run ./cmd/ptpatronus
# Web（:5173，代理 /api → :8088）
cd frontend && npm install && npm run dev
# 移动端
cd mobile && flutter pub get && flutter run -d windows   # 或 android / macos
```

## PR 前必跑（CI 门禁同款）

```bash
# 后端
cd backend && go build ./... && go vet ./... && go test ./... -short
# 前端
cd frontend && npm run typecheck && npm test && npm run build
# 移动端
cd mobile && flutter analyze && flutter test
# 插件（新增/改动插件时）
python backend/tools/pluginctl.py validate market-plugins/<你的插件>
python backend/tools/market_plugin_regression_test.py
python backend/tools/plugin_sandbox_protocol_test.py
# 工程门禁
bash scripts/check_file_length.sh check
node scripts/check-version-sync.mjs
```

## 约定

- 单文件 ≤ 400 行、≤ 80 列（棘轮 baseline，只许减不许增）。
- 新增错误一律走 `internal/api/errcode` 注册表，禁止裸写错误串。
- 新增代码的前端文案一律走 `t()`，不要硬编码中文。
- 破坏性操作（删除/清空/导入覆盖）必须「确认 + 撤销」双护栏。
- 安全漏洞请按 [SECURITY.md](SECURITY.md) 私密报告，不要开公开 issue。

## 文档地图

完整文档按角色分组见 [README/00-项目总览.md](README/00-项目总览.md) 末尾索引。
