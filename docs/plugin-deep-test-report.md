# PTPatronus 插件深度体验测试报告（修复后复测）

测试时间：2026-11-18（Asia/Shanghai）  
测试对象：market-plugins/ 下全部 **34** 个插件  
测试方式：backend/tools/pluginctl.py 的 **mock-host 沙箱**（真实启动每个插件进程 → 等待 /health → 注入宿主上下文 → 调用 /action、/event），另加 manifest 校验、语法编译、静态安全审查与独立正确性复算。  
状态：**P1–P12 已全部修复**。四轮测试分别发现 7、2、2、1 项缺陷，均已完成修复与复测。

---

## 一、总体结果（修复后）

| 项目 | 修复前 | 修复后 |
|---|---|---|
| manifest 校验（pluginctl validate） | 34/34 | 34/34 通过 |
| Python 语法编译（py_compile） | 34/34 | 34/34 通过 |
| 沙箱 /health 启动成功 | 33/34 | **34/34** |
| 实际发起的 action/event 调用 | 85 | **89**（74 ok，15 次刻意负例） |
| 响应体内隐藏错误 | 1（正常报错） | 1（正常报错） |
| 第二轮深度复测（本地 mock 服务） | — | 11 个插件 / 34 次调用，全部通过 |

第二轮（phase-2）补测了首轮因安全考虑跳过的动作：`cloudflare-subscribe update_hosts`（真实文件写入 + 白名单）、`ptp-plex-suite status/libraries/apply`（mock Plex）、`group-chat-zone run_main/run_zm`（mock 站点），以及各纯工具插件的边界输入。

覆盖说明：为避免破坏性操作，**故意未触发**外部写操作类动作（如 reboot、port_add/delete、wifi_set、organize、add_offline、sync、auth_start 等）；这些仅做静态审查。已覆盖的动作以“读取/预览/状态/纯计算”为主，并对每个负例验证了优雅报错（无进程崩溃、无裸 traceback 外泄）。

正确性交叉验证：
- twofa-helper generate：与独立 RFC6238/HMAC-SHA1 实现逐窗口比对，一致。
- text-toolbox hash：abc 的 SHA-256/MD5 与标准值一致。
- cron-helper：修复后重新验证时区、dom/dow OR、闰日表达式，均符合标准 cron 语义。
- http-probe：用模拟 302 单元测试验证跳转目标私网校验生效。
- ptp-custom-command：验证 `python -c print(123)` 以 argv 方式执行成功（输出 123）。

---

## 二、已修复的问题

### P1 — ptp-ai-subtitle 完全无法被宿主启动（阻断级）【已修复】

根因（两处）：
1. main() 读取 os.environ.get("PORT", "0")，而宿主与 pluginctl 注入的是 PTP_PLUGIN_PORT。全仓仅此插件用错变量；写到 KV 的 __listen_port 回退在 Go 宿主中无人读取。
2. _check() 使用 str.removeprefix()，需要 Python >=3.9，而宿主环境为 3.8.6，导致 /action 处理器抛 AttributeError 断连。

修复：
- main() 改为优先读 PTP_PLUGIN_PORT，PORT 仅作手工调试回退，默认 19090，不再监听随机端口。
- _check() 改为切片写法 `raw_tok[7:] if raw_tok.startswith("Bearer ")`，兼容 3.8。

验证：修复后沙箱启动 `health=ok`，`status` 动作正常返回状态对象；此前为 `<urlopen error timed out>`。

### P2 — cron-helper 非中国时区计算错误【已修复】

修复：新增 `_zone_offset(utc_dt, tz)`，按 UTC 时刻计算目标时区偏移，覆盖 UTC±HH:MM 数字偏移、常见固定偏移，以及美国/欧洲夏令时规则；未识别时区退回 UTC，不再误按 UTC+8。主循环在 UTC 轴推进、换算本地墙上时间比较。

验证：`0 9 * * *` 在 Asia/Shanghai 返回 11-19 09:00，America/New_York 返回 11-18 09:00（按当前时刻各自正确）。

### P3 — cron-helper 日/周字段用 AND【已修复】

修复：当 dom 与 dow 同时受限时改用 OR 语义，并支持 dow=7 表示周日。

验证：`0 0 1 * 1` 返回 11-23（周一）、11-30（周一）、12-01（1 号）、12-07（周一）……符合标准 cron。

### P4 — cron-helper 稀有表达式返回空【已修复】

修复：搜索窗口由 40 万分钟扩大到 4 年；并把整天/整小时的跳跃改在“本地时间”上计算后再减回 UTC 偏移（原实现会在 UTC 00:00 对应东八区 08:00 时跳过当天凌晨匹配点）。

验证：`0 0 29 2 *` 返回 2028-02-29、2032-02-29、2036-02-29；此前为 `[]`。

### P5 — http-probe 的 SSRF 防护可被 302 跳转绕过【已修复】

修复：始终使用自定义 NoRedir opener 手动处理跳转；每一跳都重新校验 scheme/主机名，并在 allow_private_network=false 时调用 _is_private_host 拦截内网/回环目标；最多 8 跳，处理 301/302/303/307/308 及 POST→GET 降级。

验证：模拟公网 URL 302 到 `127.0.0.1:8088/admin` 的单元测试中，第二次请求未发出，直接返回“默认禁止探测内网/回环地址（含跳转目标）”。正常探测（allow_private_network=true）仍返回 200。

### P6 — ptp-custom-command 使用 shell=True【已修复】

修复：改为 `shlex.split(cmd)` + `subprocess.run(argv, shell=False, ...)`，解析失败/空命令返回明确错误；每条命令执行写入 `ptp-custom-command.exec` 审计日志；README 明示 shell 语法（管道/重定向/变量展开）不再支持。

验证：`python -c print(123)` 执行成功，stdout=123，code=0。

### P7 — cloudflare-subscribe 可写任意路径【已修复】

修复：新增 `_resolve_hosts_path()`，只允许系统 hosts 文件（/etc/hosts、C:\Windows\System32\drivers\etc\hosts）、插件自身目录，以及配置 `allowed_roots` 显式授权的目录；拒绝目录路径并跟随符号链接解析；写入前生成 `.ptp-bak` 备份，响应中返回备份路径。manifest 增加 allowed_roots 配置项，README 补充说明。

验证：`C:/secret/passwd` 被拒绝；配置 allowed_roots=C:/secret 后允许；系统 hosts 与插件目录内路径正常。

### P8 — cron-helper 对无意义表达式会挂死到宿主超时【已修复】

第二轮实测：`61 * * * *`（分钟 61 超出范围）导致 `_parse_field` 返回空集合，主循环找不到任何匹配点，仍按分钟推进 4 年（约 210 万次迭代），宿主 `/action` 调用 10s 超时。

修复：解析字段后校验任一字段为空集合即直接 `raise ValueError("Cron 表达式包含无有效取值的字段（如 61 分钟、13 月）")`。

验证：同一表达式现在立即返回 `ok:false` 与明确错误；合法表达式不受影响。

### P9 — text-toolbox hash 对不支持算法返回错误结果且误标算法【已修复】

第二轮实测：`hash` 传入 `algo=sha512` 时返回 `algo:"sha512"` 但哈希值是 **SHA-256** 的结果（`ba7816...`），属于错误结果被错误标记，用户会误以为拿到了 SHA-512。

修复：显式支持 `md5/sha1/sha256/sha512`，其他值直接报错，不再静默回退。

验证：`sha512` 现返回标准值 `ddaf35a1...54ca49f`；此前返回 SHA-256 值。

### P10 — 33 个插件对畸形 JSON 请求体会直接断开连接【已修复】

第三轮协议健壮性实测：向 `POST /action` 发送 `b'{'`，33 个插件的 handler 在 `json.loads(self.rfile.read(...))` 处抛异常并直接关闭连接（`Remote end closed connection without response`）。宿主正常调用不受影响，但这是明显的协议健壮性缺陷，也可能被本地恶意客户端用于制造异常。

修复：为 33 个插件统一加入 `_safe_json_body(raw)`，解析失败返回 `{}`，handler 继续正常响应；`ptp-ai-subtitle` 原本已有 try/except，保持不变。

验证：34 插件 × 6 项协议检查（无鉴权 health、未知 GET 路径、错误 token、畸形 JSON、未知 action、未知 event）共 204 项全部通过，且每次检查后插件进程仍存活。

### P11 — ptp-ai-subtitle 未实现 `/event` 端点【已修复】

外部插件 HTTP 协议约定 `GET /health`、`POST /action`、`POST /event`。该插件缺失 `/event`，收到事件推送时返回 404。已补上空实现返回 `{"ok": true}`。

### P12 — 4 个插件声明的权限不足，宿主调用会被拒绝【已修复】

第四轮静态核对“插件实际调用的宿主 API”与 manifest 权限声明（注意：mock host 授予 `host:*`，会掩盖此类问题，必须静态检查）。发现 4 处：

- `auto-speed`：调用 `kv_set()` 但未声明 `kv:write`
- `cloudflare-subscribe`：`hosts_path` 留空时调用 `kv_set()`，未声明 `kv:write`
- `event-audit`：`emit_test_event` 调用 `publish_event()`，未声明 `event:publish`
- `webhook-transformer`：事件命中时调用 `publish_event()`，未声明 `event:publish`

按宿主 `external_host.go` 的权限要求补齐声明。验证：18 项回归测试中新增“权限声明覆盖实际宿主调用”检查，34 个插件全部通过。

补充核对：各 manifest 的动作均未声明 `input_schema`（共 123 个动作），前端按设计使用自由 JSON 输入，不存在表单字段与代码读取不一致的问题；若干插件“声明了但看似未用”的权限实际经 `/tools/{name}` 工具 RPC 使用（如 `site_checkin_all`、`tgbot_status`），不属于过度授权。

---

## 三、观察项加固（已处理）

- **管理员配置型 URL 无私网校验**：`auto-speed`（test_url）、`cloudflare-subscribe`（subscribe_urls）、`ptp-plex-suite`（base_url）新增 `_guard_target_url()`，默认拦截私网/回环/链路本地/保留地址与非 HTTP(S) 协议；局域网部署需显式开启新增配置 `allow_private_network`。manifest 与 README 已同步。
- **路由器类插件**：`ptp-miwifi`、`xiaomi-router` 本就面向内网路由器，保留 RFC1918 访问，但新增 `_assert_router_target()` 拦截 `169.254.169.254` 等链路本地/保留地址及非 HTTP(S) 协议。
- **前端定时提示**：插件卡片在“插件已启用但所有调度 cron 为空”时显示黄色「未配置定时」徽标（含 tooltip）。逻辑抽到 `frontend/src/views/pluginsHelpers.ts` 并新增 5 项单测；已通过 typecheck、vitest 与 `npm run build` 重建 Web 产物。
- **group-chat-zone**：README 补充 Cookie 明文存储与 post_url 直接 POST 的安全说明。

## 四、剩余观察项（非缺陷/非阻断）

- **定时默认关闭**：约 12 个插件的调度项 cron 默认为空串（按设计即停用）。前端现已显示「未配置定时」徽标提示，属预期行为而非缺陷。
- **管理员配置型 URL**：已按上一节加固，默认拦截私网、可显式放行；路由器类保留内网能力但拦截元数据地址。
- **文档缺失已补齐**：ptp-ai-subtitle、ptp-cd2-assistant、ptp-ffmpeg-thumb、ptp-library-scrape 4 个插件此前无 README.md，现已按各自 manifest 与实现补齐（依赖、动作、配置、权限）。
- **版本/命名不一致**：group-chat-zone 版本 3.0.0 但 manifest 名称仍为“群聊区”，动作是向配置 URL POST 消息（name|post_url|cookie|message）；README 已补充安全说明。

---

## 五、测试资产

- **已固化的回归测试（纳入版本控制）**：`backend/tools/market_plugin_regression_test.py`（18 项快速回归，覆盖 P1–P12，已接入 `.github/workflows/ci.yml` 的 plugin-sdk 任务）
- 结果 JSON：tmp/plugin-deep-results.json（修复后 89 次调用）
- 各插件运行日志：tmp/plugin-test-logs/<plugin_id>.log
- **已固化的沙箱测试（纳入版本控制）**：`backend/tools/plugin_sandbox_smoke.py`（34 插件全量冒烟）、`backend/tools/plugin_sandbox_deep_test.py`（本地 mock 服务 + 边界输入）、`backend/tools/plugin_sandbox_protocol_test.py`（34 插件 × 6 项协议健壮性检查）
- 临时运行产物：tmp/plugin_deep-results.json、tmp/plugin-phase2-results.json、tmp/plugin-test-logs/（均在 .gitignore 中）
- 专项验证：tmp/verify_totp.py、tmp/verify_cron.py、tmp/verify_ssrf.py、tmp/verify_hosts_path.py

---

## 六、结论

34 个插件全部通过 manifest 校验、语法编译与沙箱健康检查；89 次调用中 74 次正常、15 次为预期负例，纯计算逻辑经独立复算正确。

四轮测试共发现 12 项问题（P1–P12），已全部修复并复测通过：首轮 7 项，第二轮 2 项，第三轮 2 项，第四轮 1 项（权限声明缺失）。当前 34 个插件全部健康，89 次冒烟调用、204 项协议检查与 18 项回归测试全部通过，无已知阻断问题。

为防止回归，已将测试能力固化：15 项快速回归测试接入 CI，两套沙箱测试脚本纳入版本控制，后续改动插件可一键复验。剩余的观察项（URL 私网校验、前端未配置定时提示、Cookie 安全说明）也已一并处理并验证。
