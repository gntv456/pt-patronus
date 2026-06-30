# 站点定义迁移指南（PT-depiler → PTPatronus YAML）

PT-depiler（`F:\PT-depiler-master`）有 **286 个站点定义**（TypeScript），本目录用 **YAML**。
本指南说明如何把它们批量转换为 PTPatronus 的 `SiteMetadata` YAML 格式，实现「新增站点零代码」。

## 格式映射

PT-depiler 每个 `src/packages/site/definitions/<site>.ts` 导出 `siteMetadata`（实现 `ISiteMetadata`）。
对应到本项目的 `SiteMetadata`（见 `internal/site/types.go`）：

| PT-depiler (TS)            | PTPatronus (YAML)              | 说明 |
|----------------------------|--------------------------------|------|
| `id` / `name`              | `id` / `name`                  | 站点标识/显示名 |
| `schema`（NexusPHP/...）   | `schema`                       | 平台，决定走哪个 adapter |
| `urls` / `legacyUrls`      | `urls`                         | 主域名列表 |
| `search.requestConfig.url` | `search_entry` + `search_param`| 搜索入口路径与关键字参数名 |
| `search.selectors.rows`    | `selectors.rows`               | 种子行选择器 |
| `search.selectors.fields.*`| `selectors.fields.*`           | title/detail/download/size/seeders/leechers/completed/time/category |
| `userInfo.selectors.*`     | `user_info.fields.*`           | name/level/uploaded/downloaded/ratio/seeding/bonus |
| `noLoginAssert`            | `no_login`                     | 登录态断言（httpStatus/urlPattern/selector/content）|
| `levelRequirements`        | `user_info.vip_levels`         | 好学 VIP 判定用：列为 VIP 的等级名 |

## 转换步骤（批量）

1. **导出 PT-depiler 元数据为 JSON**：在 PT-depiler 工程里跑一个脚本，把每个 definition 的 `siteMetadata`
   序列化成 JSON（`search.selectors`、`userInfo`、`noLoginAssert` 等字段，多数是纯数据，少量带函数的需人工处理）。
2. **写转换器**（Node/Python/Go 均可）：读 JSON，按下表映射输出 YAML。`ElementQuery` 字段直接对应：
   ```yaml
   selectors:
     rows: { selector: "table.torrents tr" }
     fields:
       title: { selector: "a[href^='details.php']" }
       detail: { selector: "a[href^='details.php']", attr: href }
       download: { selector: "a[href^='download.php']", attr: href }
       size: { selector: "td:nth-child(5)" }
       seeders: { selector: "td:nth-child(6)" }
   ```
3. **落到本目录**：每个站点一个 `<id>.yaml`，启动时 `site.Registry.LoadDefinitions` 自动加载。
4. **人工校准**：含动态方法（`parseTorrentRowFor*`、`requestConfigTransformer`）的少数站点需手写，
   或等本项目为其 schema 增强选择器引擎后纳入。

## 平台 schema 覆盖

本项目已实现 `nexusphp` schema（HTML 抓取，覆盖绝大多数国内站）。其它 schema 待补：

- `mteam`：MTeam 走 JSON API（`api.m-team.cc` + apiKey），需独立 adapter。
- `unit3d` / `gazelle` / `tnode` / `rousi`：PT-depiler 已有 schema 基类，移植即可。

## 验证

转换后，把站点加入系统（Web UI「站点」→ 添加，选对应 definition）→ 「连接测试」验证 cookie 与登录态；
→ 「搜索」验证选择器能否正确解析种子列表。选择器不对时，对照真实页面 HTML 调整 YAML（无需改代码、无需重启即可热加载）。

## 现有定义（291 个 YAML）

- 286 个自动生成自 PT-depiler（见 `backend/tools/`）：`export-definitions.mjs`（TS AST 提取）→ `tools/dump/*.json` → `convert-definitions.go`（翻译）→ `tools/out/*.yaml` → 落到本目录。
  M2 已全量落地（2026/06/28），**保留 16 个手写校准站不被覆盖**（见下）。
  - `nexusphp`（165）：现有 adapter 可直接抓取；selectors/user_info 未列出的由 `EnsureDefaults` 自填校准默认。
  - `unit3d`(30) / `gazelle_jsonapi`(14) / `gazelle`(10) / `luminance`(8) / `avistaz`(4) / `empty`(38) 等：**先以元数据形式存在**，文件头标 `calibration: pending` +「无该 schema 适配器，待 M3+」；adapter 落地后自动可抓。
  - 死站（`dead: true`，70，跨 schema）：`Registry.ActiveDefinitions()` 已排除，不参与活跃抓取，仅保留信息可展示。
  - 含 `elementProcess`/`case`/匿名函数的字段被跳过并记入文件头 TODO 清单（标 `calibration: pending`），待 M6 真实页面校准时人工翻译。
- 手写校准站（16 个，**勿覆盖**）：azusa/agsvpt/crabpt/cspt/cyanbug/hdarea/hdfans/hdhome/hdsky/hdtime/hxpt/keepfrds/mteam/ourbits/pthome/soulvoice —— 对照真实页面手工校准过，是各自 ID 的权威定义。
- 独立站（PT-depiler 无对应）：pterclub/ptchdbits/ssd/shike —— 手写。
- `nexusphp_template.yaml`：通用 NexusPHP 模板（复制改 id 即可新增相似站）。

### 重新生成 / 增量落地

```bash
cd backend
node tools/export-definitions.mjs            # F:/PT-depiler-master → tools/dump/*.json
go run ./tools/convert-definitions.go        # tools/dump → tools/out/*.yaml
go run ./tools/verify                        # 校验 tools/out 全绿 + schema 分布
# 落地（自动跳过已存在校准站，不覆盖）：
for f in tools/out/*.yaml; do id=$(basename "$f" .yaml); [ -f "definitions/$id.yaml" ] || cp "$f" "definitions/$id.yaml"; done
go run ./tools/verify ./definitions          # 校验最终 definitions 全绿
```

> 落地规则刻意用「definitions/ 已存在同名则跳过」而非硬编码白名单——未来新增校准站天然受保护。若需用生成版覆盖某校准站，手动 `cp tools/out/<id>.yaml definitions/` 即可。
