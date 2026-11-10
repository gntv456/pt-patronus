# PTPatronus 浏览器扩展（Chrome / Edge MV3）

对标 PT-depiler、MoviePilot-Tools、MSaber、CookieCloud：跨站流式搜索、站点数据仪表（JSON/CSV/PNG 导出）、下载任务管理（剩余空间）、Cookie 台账（多域名归并 + 状态筛选）、TOTP 验证器 + 加密备份、本地凭据库（Bitwarden 导入 + 登录页自动填充）、AI 助手（LLM 网关流式对话）、插件管理（启停 + 视图嵌入）、WebDAV 全量备份、CookieCloud 协议互通、跨站收藏与下载历史、隐私模式、拖放链接衔接、豆瓣/IMDb 注入、omnibox。

## 设计语言 v1.03.19（成熟工具型）

对齐一线开发工具（VS Code / Linear / GitHub Dark）的克制风格：

- **中性深色**：slate 基底（`#0f1115` / `#161a21`），表面扁平、1px 边框分层，无渐变、无辉光、无装饰性 blob
- **单一交互色**：品牌蓝 `#4c8dff` 承担全部交互（按钮/链接/进度/焦点环）；成功绿 / 警告黄 / 危险红三语义色按需出现
- **紧凑密度**：8px 间距栅格、6-10px 圆角、12.5px 基准字号、5-6px 表格行内边距——一屏装下更多行
- **等宽数字**：表格数值 `tabular-nums` + monospace，纵向对齐可比
- **轻反馈**：toast 右上角进出场；骨架屏加载占位；悬停只做背景微亮，不做位移

## 单控制台架构（⭐ v1.03.19 重构）

选项页升级为**全功能控制台**：左侧分组导航（连接 / 站点 / 下载 / 工具 / 设置）一键切换
账号登录、站点数据、Cookie 台账、下载任务、收藏与历史、两步验证器、凭据、插件、AI 助手、同步备份、关于——
不再弹出独立小窗，所有工具在同一页面内按需懒加载。

### 凭据管理（⭐ v1.03.19 新增，对标竞品凭据库）
- 网站账号密码本地加密保管：PIN（PBKDF2-SHA256 ×310k → AES-256-GCM）后存 `chrome.storage.local`，与 TOTP 保险库同一套模式、独立 PIN
- 条目增删/编辑/复制用户名/复制密码；自由分组（内网 / PT / 自定义）+ 搜索框过滤
- **Bitwarden JSON 导入**（明文导出格式）：folders → 分组，login.totp 一并带入，按标题+URL 去重合并
- **登录页自动填充**：检测到「用户名 + 密码」表单时按 hostname 匹配（精确 > 后缀域），保守填充（不覆盖已输入内容），品牌蓝描边提示
- 锁定语义与 TOTP 一致：PIN 锁定态不后台解密；解锁后明文只进 `storage.session`（浏览器退出即清）

### AI 助手（⭐ v1.03.19 新增）
- 走自建服务器的 LLM 网关（`POST /api/v1/openai/v1/chat/completions`，OpenAI 兼容协议，SSE 流式渲染）
- Access Token 与好学登录令牌无关（服务器「AI 网关」设置里生成）；模型名可留空用网关默认
- 会话仅存页面内存，关闭即清

### 插件管理（⭐ v1.03.19 新增）
- 控制台「插件」页：服务器已装插件列表（名称/版本/启停状态）
- 一键启用/停用（管理员；`POST /plugins/:id/{enable,disable}`）
- 带 `contributes.views` 的插件出现「◨ 视图名」按钮——点击后**扩展内全屏 iframe 嵌入服务器插件页面**
  （Web 端 `/plugins/view/:id/:viewId` 路由，与 MoviePilot-Tools 的插件嵌入同形态；登出态时按服务器页面自身引导登录）

### Cookie 台账 · 状态筛选（⭐ v1.03.19 新增）
- segmented 筛选：全部 / 一致 / 不一致 / 未登录，各档带计数徽标
- 行首 2px 语义色条（绿=一致、黄=差异/缺失）一眼扫出问题站

### 站点数据 · PNG 导出（⭐ v1.03.19 新增）
- 汇总卡 + 明细表离屏 Canvas 渲染为 1080px 宽 PNG（对标竞品图片导出）
- JSON / CSV（带 BOM，Excel 直开）沿用

### 收藏与历史 · 清空（⭐ v1.03.19 新增）
- 工具栏一键清空当前列表（confirm 保护）

### TOTP · 自动解锁（⭐ v1.03.19 新增）
- 无 PIN 明文库打开控制台时自动进入列表，少一次点击

## 功能总览

### Cookie 自动同步 · 保活闭环
- **→ 上行**：浏览器启动 + 每 4 小时，插件把浏览器侧指纹有变化的站点 cookie 自动推给服务器——你刚登录过的站即刻被服务器接管保活（服务端每小时 KeepAlive 刷新 session 并回写轮换 cookie，不再依赖浏览器开着）
- **← 下行**：浏览器侧 cookie 失效/缺失的站，自动把服务器保活中的 cookie 写回浏览器（需管理员登录；非管理员静默跳过，可在 Cookie 台账页手动操作）
- **兜底通知**：两侧都无 cookie 的站才提醒重新登录（当天去重，防通知轰炸）
- 控制台「同步与备份」页可开关 + 「⟳ 立即同步」+ 最近同步结果（推送/回写/失效站数）
- 同步范围仅限服务器已录入的站点名单，不做全浏览器 cookie 收集

### 跨站找同种 · 辅种
- 收藏与历史页每条记录「🌱 找同种」：调服务端 `POST /keepseed/find` 跨站搜索并按 **名称/大小/TMDB/IMDb 指纹评分**（≥60 收录）
- 候选层列出各站同种（站点/匹配分/做种数），一键 🌱 辅种（推送到下载器）或跳详情页

### 验证码 OCR 自动识别
- 登录页检测到验证码图片自动截取 → 经服务端 `POST /system/ocr/recognize` 识别 → 回填输入框
- 识别在服务端（管理员配置 PaddleOCR/simple/ddddoc 端点），扩展零模型资产；未配置时静默回退人工输入

### 备份与同步
- **WebDAV 全量备份**：服务器配置 + API Key + TOTP 库 + 收藏，口令 AES-GCM 加密后 PUT 单文件；一键拉取恢复
- **CookieCloud 协议互通**：站点 cookie 台账以 CookieCloud 格式推送/拉取，与官方扩展/自建服务器互为备份源
- **隐私模式**：指纹、魔力、站点名/域名自动打码，防截图泄露；导出数据同样应用

### 流式跨站搜索
- popup 搜索 **SSE 流式渲染**（`/api/v1/search/stream`）：每站完成即显示
- 状态行实时显示「N 条结果 · M 站 · K 站失败」
- **omnibox**：地址栏输 `ptp` + Tab 直接跨站搜索

### TOTP 两步验证器
- 登录页检测到 2FA 输入框自动填入当期码（青柠描边提示）
- Secret 只存在验证器/后台；PIN 锁定态不后台解密
- 加密导出 / 导入：PBKDF2 ×310k → AES-256-GCM；按 secret 去重合并

### 种子页增强
- 详情页浮钮「⬇ 推送到 PTPatronus」（download.php/unit3d/magnet/eD2k）
- 列表页批量工具栏：全部推送 / 复制链接 / 收藏整页
- 免费/促销高亮（Free 绿 / 2x 紫 / 百分比金）

### 一键签到（手动触发）
- popup「📅 签到」：POST `/sites/checkin` 全站并发签到，完成后显示成功数
- 刻意不做自动定时签到（行业共识视为作弊）

## 安装（开发者模式）

1. 打开 `chrome://extensions`，右上角开「开发者模式」
2. 「加载已解压的扩展程序」→ 选择本 `extension/` 目录
3. 点扩展图标 ⚙ → 填 PTPatronus 地址（如 `http://192.168.1.10:8088`）→「账号登录」输本地用户名/密码
4. 左侧导航进入各工具页：站点数据 / Cookie 台账 / 下载任务 / 收藏与历史 / 两步验证器

## 鉴权与安全

- 与 Web/移动端同一套本地账号体系（`POST /auth/local/login`）；成功后全部请求带 `Authorization: Bearer` 直连你自己的服务器
- JWT 过期自动滑动续期（`POST /api/v1/auth/refresh`）；续期失效才用本机密封保存的本地账号凭据静默重登
- JWT、登录用户信息只存 `chrome.storage.local`（仅本机），账号密码经 `secrets.js` 密封存储；服务器地址存 `chrome.storage.sync` 跨设备同步
- 按登录身份（`user.is_admin`）自动降级：非管理员不可「← 覆盖浏览器」（cookie 明文回写）、验证码 OCR、CookieCloud 明文推送

## 文件

- `manifest.json` MV3 清单（popup + module SW + 3 组 content script + omnibox + 命令）
- `theme.css` 设计系统 v1.03.19（中性深色 / 单交互色 / 紧凑密度，全页面共用）
- `popup.html/js` 弹窗：流式搜索、数据芯片行、紧凑导航、一键签到
- `background.js` 右键菜单 + omnibox + TOTP 自动填充 + 种子推送下载（含历史）+ 收藏写入
- `content_torrent.js` 种子页增强：浮钮 / 批量工具栏 / 免费高亮
- `content_totp.js` TOTP 自动填充探测；`content_ocr.js` 验证码截取；`content_creds.js` 凭据自动填充探测
- `collection.js` 跨站收藏 + 下载历史存储层
- `credentials.js` 凭据库数据层（加密存取 / Bitwarden 解析 / hostname 匹配）
- `options.html/js` **全功能控制台**：账号登录 + 站点数据 + Cookie 台账（筛选）+ 下载任务 + 收藏历史 + TOTP + 凭据 + 插件 + AI 助手 + 同步备份 + 关于（全部侧边栏内懒加载）
- `dashboard.js` 站点数据聚合；`downloads.js` 下载器操作 + 找同种；`cookies.js` cookie 台账数据层；`totp.js` TOTP 生成器；`backup.js` WebDAV/CookieCloud；`autosync.js` cookie 自动同步；`privacy.js` 打码；`api.js` 请求封装 + SSE；`util.js` 格式化工具
