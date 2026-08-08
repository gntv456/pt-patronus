# PTPatronus Docker 容器 UI 对标设计（参考飞牛/绿联/极空间）

> 日期：2026-08-08 · 状态：**已实现**（P0+P1 全部完成，P2 模板库已完成）
> 范围：`OpsCenter.vue` → `OpsDocker.vue` 及其子组件的 UI 改进
> 对标产品：飞牛 fnOS · 绿联 UGOS Pro · 极空间 ZSpace
> 来源：三款 NAS Docker 管理界面截图分析 + 1Panel/Portainer 交互参照

---

## ★ 实现完成清单（2026-08-08）

以下改进已全部落地，前后端编译通过，测试通过：

| 改进 | 对标 | 文件 | 状态 |
|------|------|------|------|
| 创建容器多步向导（6步：镜像→基础→网络→存储→环境→确认） | 绿联 | `DockerCreateWizard.vue` | ✅ |
| 应用模板库（9个PT常用应用一键填充） | 极空间 | `dockerTemplates.ts` | ✅ |
| 端口/卷/环境动态增删行 | 绿联/极空间 | `DockerCreateWizard.vue` | ✅ |
| CPU/内存限制、网络模式、特权模式 | 绿联/极空间 | 后端 `docker_container_create.go` + `docker.ts` | ✅ |
| 容器详情分区Tab（基本信息/网络/存储/环境/日志/终端/原始配置） | 绿联/极空间 | `DockerDetailDrawer.vue` | ✅ |
| 资源监控 sparkline 折线图（CPU/内存实时） | 绿联 | `OpsDocker.vue` + 复用 `Sparkline.vue` | ✅ |
| 卡片/列表双视图切换 | 极空间 | `OpsDocker.vue` | ✅ |
| 端口协议支持 TCP/UDP | — | 前后端 | ✅ |

---

## 一、三款 NAS Docker UI 设计特点

### 1.1 飞牛 NAS（fnOS / fnos-trime）

**核心理念：卡片化 + 信息适中密度，操作触手可及**

| 区域 | 设计 |
|------|------|
| 概览栏 | 顶部 5 个彩色数字卡：运行中 / 容器总数 / 镜像 / 卷 / 网络，每卡左侧色条 + 图标方块 |
| 容器列表 | **卡片网格**（auto-fill 280px），每张卡片一个容器 |
| 卡片内容 | 状态圆点 + 容器名（粗体截断）+ 状态标签 chip；镜像名（等宽灰字）；端口映射（蓝色背景 chip）；CPU/内存占用（等宽数值）；运行时长；底部一排图标操作按钮 |
| 操作按钮 | 启动 ▶ / 停止 ⏹ / 重启 ↻ / 终端 >_ / 日志 📋 / 详情 ℹ / 删除 🗑 |
| 镜像管理 | 独立 Tab，简洁表格：仓库名 / 标签 chip / ID（等宽）/ 大小 / 删除按钮 |
| Compose | 独立「项目」入口，可视化管理 docker-compose.yml |
| 交互细节 | 卡片 hover 上浮 + 阴影加深；左侧色条按状态变色（绿=运行/灰=停止/橙=暂停/红=死亡） |

**亮点**：信息密度刚好，新手能一眼看懂每个容器在干什么。

---

### 1.2 绿联 NAS（UGREEN / UGOS Pro）

**核心理念：向导式创建 + 详情分区 Tab + 资源监控图表**

| 区域 | 设计 |
|------|------|
| 容器列表 | 列表行为主（左状态灯 + 容器名 + 镜像 + 右侧操作），紧凑 |
| 创建容器 | **多步骤向导**：① 选择镜像 → ② 基础配置（名称/命令）→ ③ 网络端口 → ④ 存储卷挂载 → ⑤ 环境变量 → ⑥ 确认摘要 |
| 镜像仓库 | **应用商店风格**：可浏览/搜索 Docker Hub 公共镜像，卡片展示，一键拉取带进度条 |
| 容器详情 | **分区 Tab**：基本信息 / 网络 / 存储 / 环境 / 日志 / 终端，信息结构化不堆 JSON |
| 资源监控 | CPU/内存**折线图**实时刷新（非静态数值） |
| 创建字段 | 丰富：CPU 限制（核数）、内存限制（MB）、特权模式、网络模式（bridge/host/none/自定义）、重启策略下拉 |

**亮点**：向导式创建降低门槛，详情 Tab 让信息可读性远超 JSON dump。

---

### 1.3 极空间 NAS（ZSpace）

**核心理念：双视图切换 + 应用模板库 + 视觉化卡片**

| 区域 | 设计 |
|------|------|
| 视图切换 | 右上角**卡片/列表双视图**按钮（⊞ 网格 / ☰ 列表） |
| 卡片设计 | 更视觉化：左侧大图标方块（应用 icon，非通用容器图标），容器名醒目，状态标签 |
| 应用模板 | **预置模板库**：qBittorrent / Jellyfin / Emby / Alist / Navidrome… 常用应用一键部署，填表即用 |
| 创建表单 | 字段全：CPU 限制、内存限制、特权模式、网络模式选择、端口/卷/环境动态增删行 |
| 操作面板 | 启动/停止/重启/删除/**克隆**/**编辑配置**（可修改已建容器的部分参数） |

**亮点**：模板库让小白零门槛部署常见应用；双视图满足不同偏好。

---

### 1.4 横向对比矩阵

| 维度 | 飞牛 | 绿联 | 极空间 | PTPatronus 现状 |
|------|------|------|--------|----------------|
| 容器列表形态 | 卡片网格 | 列表行 | 卡片/列表双视图 | ✅ 卡片网格（已对标飞牛） |
| 概览数字卡 | 5 彩色卡 | 状态汇总 | 状态汇总 | ✅ 已有（对标飞牛） |
| 创建容器 | 单页表单 | **多步向导** | 单页丰富表单 | ⚠ 单页表单（字段少） |
| 容器详情 | 抽屉 JSON | **分区 Tab** | 分区 Tab | ⚠ 抽屉 JSON dump |
| 镜像管理 | 表格 | **应用商店** | 表格+模板库 | ⚠ 纯表格 |
| 资源监控 | 卡片内数值 | **折线图** | 卡片内数值 | ⚠ 卡片内静态数值 |
| 视图切换 | 无 | 无 | **卡片/列表** | ❌ 无 |
| Compose 项目 | ✅ | ✅ | ✅ | ✅ 已有 |
| CPU/内存限制 | — | ✅ | ✅ | ❌ 创建时不支持 |
| 网络模式选择 | — | ✅ | ✅ | ❌ 仅 bridge 默认 |
| 应用模板库 | — | — | **✅** | ❌ 无 |
| 终端 exec | ✅ | ✅ | ✅ | ✅ 已有 |

---

## 二、当前实现盘点

### 2.1 已有组件清单

| 文件 | 职责 | 行数 | 状态 |
|------|------|------|------|
| `components/ops/OpsDocker.vue` | 主面板：概览+Tab+容器卡片+镜像/卷/网络表格 | 483 | 核心组件 |
| `components/ops/DockerCreateDialog.vue` | 创建容器单页表单 | 57 | **待增强** |
| `components/ops/DockerExecTerminal.vue` | exec 终端 | — | ✅ |
| `components/ops/DockerComposeLocal.vue` | 本机 Compose 部署 | — | ✅ |
| `components/ops/ComposeProjects.vue` | Compose 项目管理 | — | ✅ |
| `components/ops/useDockerOps.ts` | 拉取/清理/卷网络操作组合式 | — | ✅ |
| `views/nas-shared/opsDocker.css` | 样式（含飞牛卡片风格） | 248 | **待扩展** |
| `api/docker.ts` | Docker API 封装 | — | 需扩展字段 |

### 2.2 已对标飞牛的部分（做得好的）

- ✅ 顶部 5 彩色概览卡（运行中/总数/镜像/卷/网络）
- ✅ 容器卡片网格（auto-fill 280px）
- ✅ 卡片内容完整：状态点 + 名 + chip + 镜像 + 端口 + CPU/内存 + 操作按钮排
- ✅ 状态筛选 Tab（全部/运行中/已停止 + 计数）
- ✅ 搜索框（容器名/镜像）
- ✅ 左侧色条按状态变色

### 2.3 待改进的差距（对标三款 NAS）

| # | 差距 | 对标 | 优先级 | 影响 |
|---|------|------|--------|------|
| G1 | 容器详情是 JSON dump，不可读 | 绿联/极空间 分区 Tab | **P0** | 体验硬伤 |
| G2 | 创建容器字段太少（无 CPU/内存限制、网络模式、特权） | 绿联/极空间 | **P0** | 功能缺失 |
| G3 | 无卡片/列表双视图切换 | 极空间 | P1 | 偏好适配 |
| G4 | 资源监控是静态数值，无折线图 | 绿联 | P1 | 运维体验 |
| G5 | 无应用模板库（常用 PT 工具一键部署） | 极空间 | P2 | 锦上添花 |
| G6 | 镜像管理是纯表格，无仓库浏览 | 绿联应用商店 | P2 | 锦上添花 |
| G7 | 无法编辑已建容器配置（只能删了重建） | 极空间 | P2 | 进阶需求 |

---

## 三、改进设计方案

### 3.1 P0-A：容器详情分区 Tab（对标绿联/极空间）

**现状**：`showInspect()` 打开抽屉，`<pre>` 直接 dump `JSON.stringify(inspectData)`。

**目标**：改为结构化 Tab 展示。

```
┌─────────────────────────────────────────────┐
│  容器详情: qbittorrent                    ✕  │
├─────────────────────────────────────────────┤
│ [基本信息] [网络] [存储] [环境] [日志] [终端] │
├─────────────────────────────────────────────┤
│                                              │
│  ── 基本信息 ──                              │
│  容器ID    a3f2e1c8...                       │
│  镜像      linuxserver/qbittorrent:latest    │
│  状态      ● 运行中  (Up 3 hours)            │
│  创建时间  2026-08-07 14:22                  │
│  重启策略  unless-stopped                    │
│  命令      /init                              │
│                                              │
│  ── 资源占用 ──                              │
│  CPU  12.3%   ████████░░░░░░                 │
│  内存 256MiB / 2GiB  ██████░░░░░░ 12.5%      │
│                                              │
└─────────────────────────────────────────────┘
```

**Tab 内容映射**（从 `inspectData` 提取）：

| Tab | 数据来源 | 展示 |
|-----|---------|------|
| 基本信息 | `Id` / `Name` / `Config.Image` / `State.Status` / `Created` / `HostConfig.RestartPolicy` / `Config.Cmd` | 键值对 + 状态标签 |
| 网络 | `NetworkSettings.Networks` | 网络名/IP/网关 表格 |
| 存储 | `Mounts[]` | 类型/源路径/目标路径/模式 表格 |
| 环境 | `Config.Env[]` | KEY=value 列表 |
| 日志 | 调 `dockerApi.logs()` | 终端风格 pre，带自动刷新 |
| 终端 | 复用 `DockerExecTerminal` | 内嵌而非弹窗 |

**实现**：新建 `components/ops/DockerDetailDrawer.vue`，替换现有 `inspectDrawer` 的 `<pre>`。

---

### 3.2 P0-B：创建容器多步向导（对标绿联）

**现状**：`DockerCreateDialog.vue` 单页表单，仅镜像/名称/端口/环境/卷/命令/重启策略。

**目标**：改为 el-steps 多步向导，补齐字段。

```
步骤指示器：① 镜像 ─── ② 基础 ─── ③ 网络 ─── ④ 存储 ─── ⑤ 环境 ─── ⑥ 确认

┌── ③ 网络配置 ────────────────────────────────┐
│                                               │
│  网络模式  [bridge ▾]  bridge/host/none/自定义 │
│                                               │
│  端口映射                                     │
│  ┌─────────┬──────────┬──────┬──┐            │
│  │主机端口  │容器端口   │协议   │ ✕│           │
│  ├─────────┼──────────┼──────┼──┤            │
│  │ 8080    │ 8080     │ tcp ▾│ ✕│            │
│  │ 6881    │ 6881     │ udp ▾│ ✕│            │
│  └─────────┴──────────┴──────┴──┘            │
│  [+ 添加端口映射]                              │
│                                               │
│                          [上一步]  [下一步]    │
└───────────────────────────────────────────────┘
```

**新增字段**（对标绿联/极空间）：

| 步骤 | 现有字段 | 新增字段 |
|------|---------|---------|
| ① 镜像 | 镜像名 | （可加：从已拉取镜像选择下拉） |
| ② 基础 | 名称、命令 | **CPU 限制**（0.5~16 核）、**内存限制**（MB）、**特权模式**（switch）、**时区**（TZ 环境变量快捷注入） |
| ③ 网络 | 端口（文本域） | **网络模式**下拉、端口改为**动态增删行**（主机端口/容器端口/协议） |
| ④ 存储 | 卷（文本域） | 改为**动态增删行**（主机路径/容器路径/模式 ro-rw） |
| ⑤ 环境 | 环境变量（文本域） | 改为**动态增删行**（KEY/VALUE） |
| ⑥ 确认 | — | **摘要预览**（展示所有配置），一键创建 |

**API 扩展**（`docker_create_handler.go` / `docker.ts`）：

```go
// docker_container_create.go 新增字段
type CreateRequest struct {
    // ... 现有字段 ...
    CPULimit      float64 `json:"cpu_limit"`       // 核数，如 1.5
    MemoryLimitMB int64   `json:"memory_limit_mb"` // 内存上限
    Privileged    bool    `json:"privileged"`      // 特权模式
    NetworkMode   string  `json:"network_mode"`    // bridge/host/none
}
```

---

### 3.3 P1-A：卡片/列表双视图切换（对标极空间）

**目标**：容器 Tab 右上角加视图切换按钮。

```
搜索框  [全部|运行中|已停止]     [⊞ 卡片] [☰ 列表]    [创建] [清理]
```

- **卡片视图**（默认）：现有 `.docker-card-grid`
- **列表视图**：紧凑表格，每行一个容器，列 = 状态点 / 名称 / 镜像 / 端口 / CPU / 内存 / 运行时长 / 操作按钮

**实现**：`OpsDocker.vue` 加 `viewMode` ref（`'card' | 'list'`），列表视图复用现有表格组件逻辑。

---

### 3.4 P1-B：资源监控迷你折线图（对标绿联）

**现状**：卡片内 CPU/内存是 `statFor()` 返回的静态字符串（`12.3%`）。

**目标**：卡片内加迷你 sparkline（最近 N 个采样点）。

```
┌────────────────────────────┐
│ ● qbittorrent    [运行中]   │
│   linuxserver/qbittorrent  │
│   📡 8080:8080  6881:6881   │
│   CPU  ▁▂▃▅▇▅▃▂  12.3%     │
│   内存 ▁▁▂▂▃▃▃▂  256MiB    │
│   Up 3 hours               │
│   ▶ ⏹ ↻ >_ 📋 ℹ 🗑         │
└────────────────────────────┘
```

**实现**：
- 轮询 `dockerApi.localStats()` / `stats()`，每容器维护一个 20 点的环形缓冲
- 复用 `components/dashboard/Sparkline.vue`（项目已有）渲染
- 采样间隔 5s，组件卸载时清理定时器

---

### 3.5 P2-A：应用模板库（对标极空间）

**目标**：创建容器入口旁加「从模板创建」，预置 PT 常用应用。

**模板示例**（存 `frontend/src/data/dockerTemplates.ts`）：

| 模板 | 镜像 | 预填端口 | 预填卷 | 预填环境 |
|------|------|---------|--------|---------|
| qBittorrent | `linuxserver/qbittorrent:latest` | 8080, 6881/tcp, 6881/udp | config, downloads | `WEBUI_PORT=8080`, `TZ=Asia/Shanghai` |
| Jellyfin | `jellyfin/jellyfin:latest` | 8096 | config, cache, media | `TZ=Asia/Shanghai` |
| Emby | `emby/embyserver:latest` | 8096, 8920 | config, media | `TZ=Asia/Shanghai` |
| Transmission | `linuxserver/transmission:latest` | 9091, 51413 | config, downloads, watch | `TZ=Asia/Shanghai` |
| Deluge | `linuxserver/deluge:latest` | 8112, 6881 | config, downloads | `TZ=Asia/Shanghai` |
| Aria2 | `p3terx/aria2-pro:latest` | 6800, 6888 | config, downloads | `SECRET=随机生成` |
| Navidrome | `deluan/navidrome:latest` | 4533 | music, data | `ND_LOGLEVEL=info` |
| Alist | `xhofe/alist:latest` | 5244 | data | — |
| Metatube | `metatube-community/metatube-sdk-go:latest` | 8080 | — | — |

**交互**：选模板 → 预填向导表单 → 用户微调 → 创建。

---

### 3.6 P2-B：镜像仓库浏览（对标绿联应用商店）—— 可选

绿联内置 Docker Hub 浏览。对 PT 工具来说**优先级低**（用户通常已知镜像名），暂列入 P2 可选。

---

## 四、分阶段实施计划

### 阶段一：P0 体验硬伤修复（建议优先）

| 任务 | 涉及文件 | 预估 |
|------|---------|------|
| 新建 `DockerDetailDrawer.vue` 分区 Tab 详情 | 新建组件 + `OpsDocker.vue` 替换 inspect 抽屉 | 中 |
| 创建容器向导化 + 补 CPU/内存/网络模式/特权字段 | `DockerCreateDialog.vue` 重构 + 后端 `docker_create_handler.go` + `docker.ts` | 大 |
| 端口/卷/环境改为动态增删行 | `DockerCreateDialog.vue` | 中 |

### 阶段二：P1 体验提升

| 任务 | 涉及文件 | 预估 |
|------|---------|------|
| 卡片/列表双视图切换 | `OpsDocker.vue` + `opsDocker.css` | 小 |
| 资源监控 sparkline | `OpsDocker.vue` + 复用 `Sparkline.vue` | 中 |

### 阶段三：P2 锦上添花

| 任务 | 涉及文件 | 预估 |
|------|---------|------|
| 应用模板库 | 新建 `dockerTemplates.ts` + 模板选择 UI | 中 |
| 镜像仓库浏览（可选） | 新建组件 + Docker Hub Search API | 大 |

---

## 五、设计规范遵循

所有改动遵循项目现有设计系统（见 `menu-redesign.md` §2 设计原则）：

- **任务驱动**：容器管理按「我要做什么」组织（部署/监控/排查）
- **上下文触达**：详情走抽屉不跳页，操作按钮在卡片内就近
- **三层访问**：高频（卡片操作）→ 中频（详情 Tab）→ 低频（inspect 原始 JSON 保留在详情末尾）
- **渐进暴露**：创建向导分步，不一次性吓退新手
- **样式一致**：沿用 `var(--pt-*)` / `var(--surface-*)` / `var(--radius-*)` / `var(--sp-*)` 设计 token
- **图标体系**：沿用项目 SVG 内联图标风格（非 Element Plus 默认图标）

---

## 六、不做什么（明确边界）

- ❌ 不自建 Docker Hub 镜像加速/代理（属网络层，非 UI 职责）
- ❌ 不做容器编排/K8s（NAS 场景用 Compose 足够）
- ❌ 不做镜像构建（Dockerfile 编辑），用户自行 CI
- ❌ 不做多节点 Swarm 集群管理（超出单 NAS 场景）

---

> 本文档为设计参考，实施时以代码评审 + 实际验证为准。
