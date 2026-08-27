# PTPatronus 浏览器扩展（Chrome / Edge MV3）

对标 MSaber 浏览器扩展：快速搜索当前选中关键词、站点快捷入口、搜索结果一键订阅。

## 功能

- **选中即搜**：在任意网页选中文字后点扩展（或右键菜单「在 PTPatronus 搜索」/快捷键 `Alt+Shift+S`），自动带词跨站搜索全部 PT 站点
- **种子结果**：弹窗内直接展示 标题 / 站点 / 大小 / 做种数 / 免费 标记
- **一键订阅**：结果页点「一键订阅」→ 自动到 TMDB/豆瓣匹配条目 → 创建订阅（后台 cron 自动追更推下载器）
- **快捷入口**：Web 搜索页 / 订阅管理 / 仪表盘 一键直达

## 安装（开发者模式）

1. 打开 `chrome://extensions`，右上角开「开发者模式」
2. 「加载已解压的扩展程序」→ 选择本 `extension/` 目录
3. 点扩展图标 ⚙ → 填 PTPatronus 地址（如 `http://192.168.1.10:8088`）与 API Key（服务端 `API_KEY` 环境变量，与 Servarr/Overseerr 接入同一把密钥）→ 测试连接 ✅

## 鉴权与安全

- 全部请求带 `X-API-Key` 头直连你自己的服务器，不经过任何第三方
- 配置存 `chrome.storage.sync`（随 Chrome 账号同步），密钥不明文出现在 URL

## 文件

- `manifest.json` MV3 清单（popup + service worker + 命令）
- `popup.html/js` 弹窗：搜索框（自动带选中词）、结果列表、一键订阅
- `background.js` 右键菜单 + 快捷键
- `options.html/js` 服务器与 API Key 配置 + 连接测试
- `api.js` 请求封装（popup/options 共用）
