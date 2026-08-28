# PTPatronus 浏览器扩展（Chrome / Edge MV3）

对标 PT-depiler、MoviePilot-Tools、MSaber、CookieCloud：跨站流式搜索、站点数据仪表（JSON/CSV 导出）、下载任务管理（剩余空间）、Cookie 台账（多域名归并）、TOTP 自动填充 + 加密备份、WebDAV 全量备份、CookieCloud 协议互通、跨站收藏与下载历史、隐私模式、拖放链接衔接、豆瓣/IMDb 注入、omnibox。

## 设计语言 · Liquid Glass（v2.1 UI 重设计）

六页面统一采用「液态玻璃」设计系统（`theme.css`）：

- **大胆撞色**：荧光青柠 `#C8FF3D` × 电光紫 `#7C5CFF` × 珊瑚橙 `#FF6B4A` × 水光青 `#3DDFFF`，铺在深海墨绿底上互相对撞，拒绝安全灰蓝
- **iOS27 磨砂玻璃**：`backdrop-filter: blur(22px) saturate(160%)` 半透玻璃层叠出「水下透光」景深，配 1px 内描边高光模拟玻璃反光
- **有机曲线**：品牌 logo 与工具图标用不对称 blob 超椭圆（`border-radius: 62% 38% 55% 45% / 45% 55% 45% 55%`），拒绝标准圆角
- **自然纹理**：多层 `repeating-radial-gradient` 细水波纹 + 角落大尺度模糊光斑（水生物）
- **水润质感**：进度条/验证码带彩色辉光（`box-shadow` 光晕）、按钮内嵌顶光+底压暗、芯片底缘撞色水流线
- **密集信息**：等宽数字（tabular-nums）、微型大写标签（0.24em 字距 eyebrow）、撞色胶囊徽标，专业终端气质

## 功能

### 跨站找同种 · 辅种（⭐ v2.5 新增）
- 收藏与历史页每条记录新增「🌱 找同种」：调服务端 `POST /keepseed/find` 跨站搜索并按 **名称/大小/TMDB/IMDb 指纹评分**（≥60 收录）
- 候选层列出各站同种（站点/匹配分/做种数），一键 🌱 辅种（推送到下载器）或跳详情页——把已在做的种在其它站也做上
- 匹配在服务端（`internal/chain/keepseed.go` 的评分引擎），扩展零指纹库成本

### 验证码 OCR 自动识别（⭐ v2.4 新增）
- 登录页检测到验证码图片（captcha/seccode/验证码 启发式）自动截取 → 经服务端 `POST /system/ocr/recognize` 识别 → 紫色描边回填输入框
- 轻量路线：识别在服务端（管理员在系统设置配置 PaddleOCR/simple/ddddoc 端点），扩展零模型资产；未配置时静默回退人工输入
- 支持「换一张」刷新后重识别；图片经用户自建服务器流转，不出内网

### 备份与同步（⭐ v2.3 新增）
- **WebDAV 全量备份**：服务器配置 + API Key + TOTP 库 + 收藏，口令 AES-GCM 加密后 PUT 单文件；一键拉取恢复
- **CookieCloud 协议互通**：把站点 cookie 台账以 CookieCloud 格式推送/拉取（`md5(uuid-password)` key + AES-128-CBC OpenSSL EVP 派生，与官方扩展/自建服务器互为备份源）；密码本地加解密不出本机
- **隐私模式**：选项页一键开关——指纹（`d927••••`）、魔力、站点名/域名自动打码，防截图泄露；导出数据同样应用

### 导出与拖放（⭐ v2.3 新增）
- 站点数据页 **📤 JSON / 📥 CSV**（带 BOM，Excel 中文直开）
- 站点页拖动磁力/种子/eD2k 链接后打开 popup，出现「拖放的链接」快捷推送条（Chrome 扩展图标不支持原生 drop，两步衔接）

### 多域名归并（⭐ v2.3 新增）
- Cookie 台账按站点名归并 CDN/备选域名，展示「+N 域」徽标

### 流式跨站搜索（⭐ v2.2 升级）
- popup 搜索改为 **SSE 流式渲染**（`/api/v1/search/stream`）：每站完成即显示，65 站不再等 30 秒白屏
- 状态行实时显示「N 条结果 · M 站 · K 站失败」
- **omnibox**：地址栏输 `ptp` + Tab 直接跨站搜索（对标 PT-depiler 的 `ptd`）

### TOTP 自动填充 + 加密备份（⭐ v2.2 新增）
- 登录页检测到验证码/2FA 输入框（二步/两步/2FA/OTP/动态码 启发式）**自动填入当期码**，青柠描边提示
- Secret 只存在验证器/后台，页面只能拿到 6 位数字本身；PIN 锁定态不后台解密（锁定语义完整）
- **📤 加密导出 / 📥 导入**：备份口令 PBKDF2 ×310k → AES-256-GCM；按 secret 去重合并；支持明文迁移格式

### 豆瓣 / IMDb 页面注入（⭐ v2.2 新增）
- 豆瓣条目页/Top250/选电影、IMDb 标题页/Top250 右下角出现「🔍 在 PTPatronus 搜」浮钮
- 自动提取片名 + 年份（清洗豆瓣 `(1993)` / IMDb `- IMDb` 后缀形态）

### 跨站收藏与下载历史（⭐ v2.2 新增）
- 种子列表页工具栏新增「⭐ 收藏本页种子」——一键收藏整页进统一收藏夹（chrome.storage.local，上限 500）
- 每次推送下载自动记历史（上限 300）；选项页「⭐ 收藏与历史」两标签管理，支持再推送/重推/删除
- 浮钮兜底识别扩展到 **eD2k 链接**（magnet 之外）

### 下载器剩余空间（⭐ v2.2 新增）
- 下载任务面板顶部显示所选下载器磁盘余量（后端 `GlobalStats.free_space`，qB `free_space_on_disk`）

### 搜索与订阅
- **选中即搜**：在任意网页选中文字后点扩展（或右键菜单「在 PTPatronus 搜索」/快捷键 `Alt+Shift+S`），自动带词跨站搜索全部 PT 站点
- **种子结果**：弹窗内直接展示 标题 / 站点 / 大小 / 做种数 / 免费 标记
- **一键订阅**：结果页点「一键订阅」→ 自动到 TMDB/豆瓣匹配条目 → 创建订阅（后台 cron 自动追更推下载器）
- **快捷入口**：Web 搜索页 / 订阅管理 / 仪表盘 / 站点数据 一键直达（未配置时自动引导去设置）

### 站点数据仪表（⭐ v2.1 新增）
- 选项页「📊 站点数据」：跨站汇总卡片（总上传/下载/分享率/做种数/做种体积/魔力值/魔力时产）+ 每站明细表（等级/上传/下载/做种/魔力，按上传量排序）
- 「⟳ 即时抓取全部站点」逐站拉取 userinfo 写快照（站均 1-3s）
- 无数据站点标红 ⚠ 提示疑似 cookie 失效
- popup 底部快捷块：总上传 / 做种 / 魔力 一眼可见

### 下载任务管理（⭐ v2.1 新增）
- 选项页「⬇ 下载任务」：多下载器切换（选择会记住，浮钮推送共用）、任务表（进度条/状态/速度/分享率）、5s 自动刷新
- 单任务 暂停/继续/删除（删任务不删文件，防误删数据）；全部暂停/继续
- 粘贴种子链接/磁力直接添加（服务端按 URL 注册域自动带站点凭据）

### 站点 Cookie 台账
- 选项页「🔑 站点 Cookie 台账」打开对比视图：服务器每个站点一行，**指纹 = cookie 的 sha256 前 12 位**（不泄露明文即可比对异同）
- 浏览器列读取本机 Chrome 中该站注册域下的会话 cookie（含 HttpOnly——扩展 cookies 权限允许），与服务器指纹比对：
  - 🟢 一致　🟡 指纹不一致　🔴 浏览器未登录 / 服务器未录入
- **→ 覆盖服务器**：把浏览器 cookie 上传为该站点凭据（顺手同步 WebView/浏览器里刚登录的号）
- **← 覆盖浏览器**：把服务器 cookie 写回浏览器 jar（明文拉取端点 `/sites/cookies/reveal` 已收敛 RequireAdmin，需管理员 API Key；写回 path=/ 且 30 天有效期，覆盖同名旧值）

### 种子页增强（⭐ v2.1 新增）
- **详情页浮钮**：右下角「⬇ 推送到 PTPatronus」，自动识别 `download.php?id=` / unit3d `/download` 链接（兜底 magnet）
- **列表页批量工具栏**（NexusPHP torrents.php / unit3d /torrents）：顶部显示「N 个种子」，一键 **全部推送下载**（逐个经服务器入列）或 **复制全部链接**
- **免费/促销高亮**：NexusPHP `pro_free` / `pro_free2up` / `pro_50pctdown` 等图标行首加彩色徽标（Free 绿色、2x 紫色、百分比金色），免费种一眼可见
- 加种带 `ptpatronus-extension` 标签便于来源追溯

### 一键签到（⭐ v2.1 新增，手动触发）
- popup「📅 一键签到」：POST `/sites/checkin` 全站并发签到，完成后显示成功数
- 刻意**不做自动定时签到**——竞品（PT-depiler）官方明示自动获取奖励类功能视为作弊，行业共识

### TOTP 两步验证器
- 选项页「🔐 两步验证器」：粘贴 `otpauth://totp/...` 链接或裸 base32 秘钥添加条目
- RFC 6238 本地生成（HMAC-SHA1，WebCrypto），**通过 RFC 官方向量测试**；30s 倒计时条形进度 + 点击复制验证码
- 可选 PIN 保护：PIN 经 PBKDF2-SHA256 ×310,000 轮派生 AES-256-GCM 密钥加密整个库；设 PIN 后每次打开需解锁，「🔒 锁定」按钮随时上锁

## 安装（开发者模式）

1. 打开 `chrome://extensions`，右上角开「开发者模式」
2. 「加载已解压的扩展程序」→ 选择本 `extension/` 目录
3. 点扩展图标 ⚙ → 填 PTPatronus 地址（如 `http://192.168.1.10:8088`）→「好学账号」tab 输好学用户名/密码（2FA/验证码按需）登录，或切「Cookie 登录」粘贴好学 Cookie（覆盖挑战响应账号）→ 显示「已登录 ✅」
4. 工具入口都在选项页底部：「📊 站点数据」「⬇ 下载任务」「🔑 站点 Cookie 台账」「🔐 两步验证器」

## 鉴权与安全

- 与 Web/移动端同一套好学账号体系：「好学账号」tab 走 `POST /api/v1/auth/hxpt/captcha` + `/auth/hxpt/login` 两步（服务端代理验证码/挑战响应），「Cookie 登录」走 `/auth/hxpt/cookie-login`（最稳，覆盖 2FA/挑战响应）；成功后全部请求带 `Authorization: Bearer` 直连你自己的服务器，不经过任何第三方
- JWT 过期自动**滑动续期**（`POST /api/v1/auth/refresh`，服务端自签自验，不打扰好学站）；仅当续期失效（登出/吊销）才用本地好学 Cookie 重新兑换；Cookie 也失效时提示重新登录
- 好学 Cookie、JWT、登录用户信息只存 `chrome.storage.local`（仅本机，不随 Chrome 账号同步）；服务器地址存 `chrome.storage.sync` 跨设备同步
- 按登录身份（`user.is_admin`）自动降级：非管理员看不到「← 覆盖浏览器」（cookie 明文回写）与验证码 OCR 识别，CookieCloud 明文推送同样仅管理员
- 服务端相关端点：`GET /api/v1/sites/cookies`（脱敏指纹，登录可用）、`GET /api/v1/sites/cookies/reveal`（明文，RequireAdmin）、`POST /api/v1/auth/hxpt/cookie-login`（公开，限流）、`POST /api/v1/auth/refresh`（需有效 Bearer）

## 文件

- `manifest.json` MV3 清单（popup + module SW + 3 组 content script + omnibox + 命令）
- `theme.css` 液态玻璃设计系统（撞色 token / 磨砂玻璃 / 有机曲线 / 水润光晕，七页面共用）
- `popup.html/js` 弹窗：流式搜索（逐站显示）、五格玻璃导航、站点数据快捷芯片、一键签到
- `background.js` 右键菜单 + omnibox + TOTP 自动填充 + 种子推送下载（含历史记录）+ 收藏写入
- `content_torrent.js` 种子页增强：详情页浮钮（含 eD2k）、列表页批量工具栏（推送/复制/收藏）、免费高亮
- `content_totp.js` TOTP 自动填充探测（验证码输入框启发式 + SPA 兼容）
- `content_media.js` 豆瓣/IMDb 页面「搜 PT 种」浮钮（片名+年份提取）
- `collection.js` 跨站收藏 + 下载历史存储层（chrome.storage.local，FIFO 上限）
- `history_page.html/js` 收藏与历史管理页（两标签，再推送/重推/删除）
- `dashboard.js` + `stats_page.html/js` 站点数据仪表（总览/即时抓取/格式化）
- `downloads.js` + `downloads_page.html/js` 下载任务面板（多下载器/暂停/继续/删除/添加/剩余空间）
- `cookies.js` + `cookies_page.html/js` 站点 Cookie 台账（注册域提取 / 会话读取 / 双向覆盖）
- `totp.js` + `totp_page.html/js` TOTP 生成器（base32/RFC6238/加密库/导入导出）；`totp_autofill_lib.js` SW 复用桥
- `options.html/js` 服务器与 API Key 配置 + 连接测试 + 撞色工具马赛克入口
- `api.js` 请求封装（好学账号登录/JWT Bearer/401 自动重登）+ SSE 流式搜索解析（全组件共用）
