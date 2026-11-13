<div align="center">

# 🛡️ PTPatronus · PT 守护神

**自托管 PT 一体化管理台** — 站点数据 · 自动签到 · H&R 告警 · 下载器 · 辅种 · 订阅 · 媒体库 · NAS 运维

[![CI](https://github.com/gntv456/pt-patronus/actions/workflows/ci.yml/badge.svg)](https://github.com/gntv456/pt-patronus/actions/workflows/ci.yml)
[![Docker](https://github.com/gntv456/pt-patronus/actions/workflows/docker.yml/badge.svg)](https://github.com/gntv456/pt-patronus/actions/workflows/docker.yml)
[![License: AGPL-3.0](https://img.shields.io/badge/License-AGPL_v3-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/backend-Go_1.26-00ADD8?logo=go&logoColor=white)](backend)
[![Vue3](https://img.shields.io/badge/web-Vue_3-4FC08D?logo=vuedotjs&logoColor=white)](frontend)
[![Flutter](https://img.shields.io/badge/mobile-Flutter-02569B?logo=flutter&logoColor=white)](mobile)
[![Release](https://img.shields.io/github/v/release/gntv456/pt-patronus?include_prereleases)](https://github.com/gntv456/pt-patronus/releases)

**Docker 一行起全功能** · 三端（Web / Android / Windows 等）· 300+ 站点定义热更新 · 中文界面

</div>

---

## ✨ 这是什么

PT 站玩家的「保号 + 追剧 + 管种」一体化平台：把散落在各 PT 站的账号数据、
下载器、媒体库和 NAS 收进一个自托管面板，自动化处理保号琐事，让你只管刷剧。

```bash
mkdir ptpatronus && cd ptpatronus
curl -O https://raw.githubusercontent.com/gntv456/pt-patronus/main/docker-compose.example.yml
docker compose up -d
# 浏览器打开 http://<NAS的IP>:8088
```

> 部署前把 compose 里的 `ADMIN_PASSWORD` 改成自己的强密码（模板已预设 admin 账号，
> 详见 [快速开始](README/快速开始.md)）。

## 🧩 核心能力

| 域 | 能力 |
|---|---|
| **站点管理** | 300+ 站点定义（NexusPHP / UNIT3D / Gazelle / 自研引擎），远端源热更新——新站适配零发版 |
| **保号自动化** | 定时签到（记录可回看）· H&R 临期告警 · Cookie 保活失效告警 · 注册开放抢号提醒 |
| **数据看板** | 上传/下载/做种/魔力值聚合与趋势，站点运营矩阵，体检分 |
| **下载器** | qBittorrent / Transmission / Deluge / Aria2：任务管理、限速、分类标签、WS 实时推送 |
| **追剧订阅** | 跨站搜索 + 订阅命中自动下载，质量评分闸门，TMDB / 豆瓣 / Bangumi 元数据 |
| **辅种** | IYUU 协议跨站辅种，一键补种提分享率 |
| **媒体库** | Emby / Jellyfin / Plex 对接，整理入库、STRM、观影报告 |
| **NAS 运维** | SSH 终端 / Docker 管理 / 文件管理（WebDAV · Alist · SMB）/ 网盘 / 监控告警 |
| **工作流引擎** | 可视化节点图编排（59 个节点），手动 / 定时 / 事件触发 |
| **更多** | 浏览器扩展 · 插件系统 · 全局通知中心（TG/微信/邮件等）· AI 助手 |

## 🖥️ 三端

| 端 | 技术栈 | 说明 |
|---|---|---|
| Web（随镜像内嵌） | Vue 3 + Element Plus | 主界面，`:8088` 即用 |
| 移动/桌面 | Flutter（Android / Windows / macOS / 鸿蒙） | 与 Web 同 token、实时推送 |
| 浏览器扩展 | Chrome MV3 | Cookie 一键入库 · 站内数据抓取（设置→关于 下载） |

## 🚀 快速开始

五步走（约 5 分钟）：**部署 → 登录 → 添加站点 → 添加下载器 → 打开仪表盘**
→ [完整图文步骤](README/快速开始.md)

<details>
<summary><b>本地开发</b>（三端起法 / 端口 / 环境变量）</summary>

```bash
# 后端（默认 :8088，SQLite 零配置）
cd backend && go run ./cmd/ptpatronus
# Web UI（:5173，代理 /api → :8088）
cd frontend && npm install && npm run dev
# Flutter 客户端
cd mobile && flutter pub get && flutter run -d windows   # 或 android / macos
```

- 首登自动建号（首个登录者即管理员）；compose 用 `ADMIN_USERNAME/ADMIN_PASSWORD` 预设
- `SECRET_KEY` 留空自动生成并持久化到 `./data`（**不要删 data 目录**——密钥丢了已加密凭据解不开）
- 可选 PostgreSQL：`docker compose --profile pg up -d`
- Windows 终端中文乱码等排障见 [启动与编码排障](README/启动与编码排障.md)

</details>

## 📖 文档

| 文档 | 内容 |
|---|---|
| [快速开始](README/快速开始.md) | 部署到上手五步走（新用户必读） |
| [05-部署指南](README/05-部署指南.md) | Docker / NAS / 反代 / 升级与回滚 |
| [04-API 参考](README/04-API参考.md) | REST / WS / 错误码约定 |
| [06-站点定义指南](README/06-站点定义指南.md) | 新站点适配（YAML 定义） |
| [07-开发指南](README/07-开发指南.md) | 架构 / 本地开发 / 测试 / 门禁 |
| [01-架构设计](README/01-架构设计.md) · [02-技术栈](README/02-技术栈与选型.md) | 深入设计 |
| [CHANGELOG](CHANGELOG.md) | 版本变更与升级注意 |
| [全部文档索引](README/00-项目总览.md) | 24 篇专题文档 |

## 🤝 参与贡献

- **适配新站点**：只需提交一份 YAML 定义到[站点定义仓库](https://github.com/gntv456/ptpatronus-sites)，
  全部实例自动热更新——见 [06-站点定义指南](README/06-站点定义指南.md)
- **报问题 / 提建议**：[Issues](https://github.com/gntv456/pt-patronus/issues)
- 开发环境与门禁（400 行/80 列棘轮、UI 可访问性、对比度）见 [07-开发指南](README/07-开发指南.md)

## 📄 许可证

[AGPL-3.0-or-later](LICENSE)（GNU Affero 通用公共许可证）。

强 Copyleft：通过网络提供本程序服务时，须向这些用户同等提供完整对应源代码。
你可以自由使用、修改、分发，但任何形式的分发（含网络服务）都须以 AGPL-3.0 继续开源你的修改。

---

<div align="center">

**⭐ 如果 PTPatronus 帮你保住了号，欢迎 Star 支持**

</div>
