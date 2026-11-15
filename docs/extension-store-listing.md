# 浏览器扩展上架指南（Chrome Web Store / Edge Add-ons）

> 适用版本：1.03.30+（manifest.json 在仓库 `extension/`，商店包即
> `backend/internal/api/v1/extension_bundle.zip` 同款形态——manifest 在 zip 根、
> 已排除 test/；导出副本：仓库根 `ptpatronus-extension-1.03.30-store.zip`）
> 注意：扩展**没有 update_url**（MV3 自托管更新需要 CRX 签名体系），上架商店后
> 浏览器才会自动更新；商店上架与「关于页下载手动装」两条分发渠道并存。

## 0. 上架前必须知道的架构事实

- 权限模型：`host_permissions` 只含豆瓣/IMDb（内容注入），**所有站点与用户自建
  服务器的访问走 `optional_host_permissions`（http://*/* https://*/*）**——用户
  首次连接服务器时经 `chrome.permissions.request` 动态授予。这是商店审核的
  重点合规项，文案里必须把「为什么需要」写清楚（见 §3 文案，直接抄）。
- `cookies` 权限用途：跨站 Cookie 台账 + 保活同步（CookieCloud 协议互通），
  仅作用于用户显式录入的站点名单，不做全浏览器 cookie 收集。
- 无远程代码（MV3 禁令）：全部 JS 本地打包，无 eval/远程脚本注入——审核友好。

## 1. Edge Add-ons（推荐先上，免费、审核快、你当前在用）

1. 注册 [Microsoft Partner Center](https://partner.microsoft.com/dashboard/microsoftedge)
   （Microsoft 账号即可，免费，需完成开发者身份验证，1-2 天）
2. 「Create new extension」→ 上传 `ptpatronus-extension-1.03.30-store.zip`
3. 填写 Metadata（直接抄 §3）：
   - Name: `PTPatronus 助手`；Short description: §3 短文案
   - Description: §3 长文案；Category: `Productivity`
   - Privacy policy URL: 必填——用仓库 README 或 `docs/privacy.md` 的 GitHub 链接
   - Website URL: 仓库地址
4. 「Privacy」步骤：勾选「Does not collect personal data」不成立（收集站点
   Cookie 到用户自建服务器）→ 按实勾选并说明：数据仅发送到**用户自己配置的
   自托管服务器**，开发者不接收任何数据
5. 提交 → 审核通常 1-7 天；`cookies`+可选权限一般会有人工复核，被拒按反馈补
   「最小权限说明」即可

## 2. Chrome Web Store（覆盖 Chrome/Brave/Opera 等 Chromium 系）

1. 注册 [Chrome Web Store 开发者账号](https://chrome.google.com/webstore/devconsole/register)
   （Google 账号 + **一次性 $5 注册费**，需信用卡/Google Pay）
2. 「New item」→ 上传同一个 zip（Chrome 与 Edge 用同一份，MV3 通用）
3. Store listing：文案抄 §3；**截图至少 1 张 1280×800 或 640×400**（建议截
   扩展控制台的站点数据仪表页与 Cookie 台账页）；图标已内置 128×128
4. Privacy tab：
   - Single purpose: 「PT 站点数据聚合与下载管理工具（连接用户自托管服务器）」
   - `cookies` 权限用途声明（§3 隐私声明段）
   - 是否远程代码：**No**（MV3 本就禁止，如实勾）
   - Privacy policy URL 必填
5. Submit for review → 深入审核 1-2 周（`cookies` 触发），
   期间可勾「扩展内 beta 测试」先发 test 版给指定用户

## 3. 商店文案（直接复制）

**Short description（≤132 字符）**：
> PT 站点助手：跨站搜索/订阅、数据仪表、Cookie 保活同步、TOTP、凭据库、辅种、AI 助手——连接你的自托管 PTPatronus

**Detailed description**：
> PTPatronus 助手是自托管 PT 工具箱 PTPatronus 的浏览器端：Cookie 自动同步保活
> （浏览器↔服务器双向，按站点名单最小范围）、跨站流式搜索与订阅推送、站点数据
> 仪表（JSON/CSV/PNG 导出）、下载任务管理、TOTP 两步验证器（登录页自动填充）、
> 本地加密凭据库（Bitwarden 导入）、验证码 OCR、跨站辅种（找同种）、插件管理、
> AI 助手、WebDAV/CookieCloud 备份、豆瓣/IMDb 页面注入、omnibox 搜索。
>
> 数据流向说明：本扩展是**你自托管服务器的浏览器端**，全部数据仅在你与你自己
> 的服务器之间传输，开发者不接收、不中转任何数据。不访问你未录入的站点。
>
> 需要：自部署的 PTPatronus 服务器（AGPL-3.0 开源，GitHub 搜 PTPatronus）。

**权限用途声明（审核回复可直接用）**：
- `cookies`：读取用户显式录入的 PT 站点 cookie，用于登录态保活与失效提醒；
  范围仅限服务器已录入名单，不做全浏览器收集
- `optional_host_permissions`：用户配置自建服务器地址后请求该域名访问权限；
  站点访问权限仅在用户触发对应功能（跨站搜索/辅种/同步）时按需请求
- `scripting/activeTab`：豆瓣/IMDb 页面注入种子条目；站点页内的辅种匹配提示
- `alarms/webNavigation`：4 小时周期保活同步与登录页凭据填充检测

## 4. Firefox（AMO）暂缓原因

manifest 用 `background.service_worker`（MV3 Chromium 形态），Firefox 需要
`background.scripts`（event page）适配 + web-ext 打包——待 Chromium 系两店
跑通后单独适配（background.js 入口导出兼容层即可，工作量约半天）。

## 5. 版本更新节奏

商店版与仓库版同步：改 `extension/` 后 ① 重跑 `python scripts/pack-extension.py`
（bundle 门禁会拦过期）② bump `extension/manifest.json` version ③ 商店后台
重新上传 zip。Edge/Chrome 商店审核通过后自动推送更新给已安装用户。
