# PTPatronus 插件深度体验测试报告（修复后复测）

测试时间：2026-11-18（Asia/Shanghai）  
测试对象：market-plugins/ 下全部 **34** 个插件  
测试方式：backend/tools/pluginctl.py 的 **mock-host 沙箱**（真实启动每个插件进程 → 等待 /health → 注入宿主上下文 → 调用 /action、/event），另加 manifest 校验、语法编译、静态安全审查与独立正确性复算。  
状态：**P1–P7 已全部修复，见第二节各项的「修复/验证」说明**。

---

## 一、总体结果（修复后）

| 项目 | 修复前 | 修复后 |
|---|---|---|
| manifest 校验（pluginctl validate） | 34/34 | 34/34 通过 |
| Python 语法编译（py_compile） | 34/34 | 34/34 通过 |
| 沙箱 /health 启动成功 | 33/34 | **34/34** |
| 实际发起的 action/event 调用 | 85 | **89**（74 ok，15 次刻意负例） |
| 响应体内隐藏错误 | 1（正常报错） | 1（正常报错） |

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

---

## 三、剩余观察项（非缺陷/非阻断）

- **定时默认关闭**：约 12 个插件的调度项只有 cron_config_key 且默认值为空串（如 auto-speed、ptp-custom-command、xiaomi-router、ptp-ai-subtitle 等）。按设计空串即停用，但用户不配置就“看起来没生效”，建议前端在插件卡片上明确提示“未配置定时任务”。
- **管理员配置型 URL 无 SSRF 校验**：auto-speed（test_url）、cloudflare-subscribe（subscribe_urls）、ptp-plex-suite（base_url）、ptp-miwifi / xiaomi-router（路由器地址）都由配置直接决定目标，未做私网校验。属于管理面能力，风险低于 P5；如插件配置权限下放需重新评估。
- **文档缺失已补齐**：ptp-ai-subtitle、ptp-cd2-assistant、ptp-ffmpeg-thumb、ptp-library-scrape 4 个插件此前无 README.md，现已按各自 manifest 与实现补齐（依赖、动作、配置、权限）。
- **版本/命名不一致**：group-chat-zone 版本 3.0.0 但 manifest 名称仍为“群聊区”，动作是向配置 URL POST 消息（name|post_url|cookie|message），建议补充说明其用途与 Cookie 明文存储风险。

---

## 四、测试资产

- 测试脚本：tmp/plugin_deep_test.py（可重复执行：python tmp/plugin_deep_test.py [plugin_id ...]）
- 结果 JSON：tmp/plugin-deep-results.json（修复后 89 次调用）
- 各插件运行日志：tmp/plugin-test-logs/<plugin_id>.log
- 专项验证：tmp/verify_totp.py、tmp/verify_cron.py、tmp/verify_ssrf.py、tmp/verify_hosts_path.py

---

## 五、结论

34 个插件全部通过 manifest 校验、语法编译与沙箱健康检查；89 次调用中 74 次正常、15 次为预期负例，纯计算逻辑经独立复算正确。

首轮发现的 7 项问题（P1–P7）已全部修复并复测通过，其中阻断性的 ptp-ai-subtitle 现已可正常启动与响应。剩余为产品体验与管理面风险提醒，不阻断使用。
