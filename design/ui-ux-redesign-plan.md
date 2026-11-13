# PTPatronus 全站 UI / 交互改版方案（去 AI 感 · 大厂成熟感）

> 版本：v1 草案 · 2026-11-13 · 范围：Web（Vue3）、Flutter 移动端、浏览器扩展
> 方法：加载 `ui-ux-pro-max` skill → 读取 `SKILL.md` / `references/quick-reference.md` / `references/pro-rules.md` → 运行 `search.py` 设计系统与域检索 → 对 47 个 Web 页面 + 25 个 Flutter 页面 + 扩展 popup/options 做源码级审计 → 汇总为可落地修改方案。
> 性质：**设计与交互方案，不改业务代码**；本文所有「现状」均带 文件:行号 证据。

---

## 0. Skill 使用记录与结论可信度

| 检索 | 查询 | 结果 | 采信情况 |
|---|---|---|---|
| `--design-system`（第 1 次） | `self-hosted media management admin console professional dense` | Style=Minimalism & Swiss；Typography=Playfair Display（编辑体）；Palette=玫红 | **部分不采信**：字体/色板与后台管理台不符，按 skill 规则重试 |
| `--design-system`（第 2 次，收窄） | `enterprise infrastructure admin console neutral utility --variance 2 --motion 2 --density 9` | Style=Minimalism & Swiss（Best For: Enterprise apps / dashboards / professional tools）；Typography=Inter + Inter（mood: minimal, swiss, functional, neutral）；Palette=`#2563EB` trust blue / `#F8FAFC` / `#1E293B` / `#E2E8F0` / `#DC2626`；Effects=Subtle hover 200–250ms | **采信** Style / Typography / Color / Motion；**不采信** Pattern（返回「Hero + Features + CTA」营销落地页模式，与数据密集型后台不符——按 skill 要求明确声明：该模式无适配性验证，未采用） |
| `--domain style` | `enterprise admin dashboard professional neutral` | **data-dense-dashboard**（已验证匹配）：12 栏网格、gap 8px、padding 12px、12–14px 字号、表格行高 36px、侧栏 240px、头栏 56px、粘性表头、可排序、加载态、导出 | **全文采信**，作为密度基准 |
| `--domain color` | `enterprise tool neutral blue gray` | Knowledge Base/Documentation：中性灰 `#475569` + 链接蓝 `#2563EB`、`#F8FAFC` 背景、`#E2E8F0` 边框 | **采信** |
| `--domain typography` | `dashboard data dense UI sans` | Dashboard Data（Fira Sans + Fira Code）/ Inter system-first；中文需 PingFang SC / Microsoft YaHei 回退 | **采信原则**，字体族按中文场景调整 |
| `--domain icons`（第 1 次 0 结果 → 收窄重试） | `icon button accessible label` | `icon-context-accessibility`：单一图标族（Phosphor/Heroicons Outline）、装饰图标 `aria-hidden`、图标按钮必须有 accessible name 并暴露 `aria-pressed/expanded` | **采信** |
| `--domain product` | `self-hosted NAS admin tool developer` | Developer Tool/IDE：Minimalism & Swiss + 深色、仪表盘=Real-Time Monitor + Terminal、蓝色焦点 | **采信方向** |
| `--domain chart` | `dashboard sparkline trend chart` | 时间序列用折线；实线/虚线/点线区分系列 + 直接标注；可见数据表兜底；键盘可聚焦取值 | **采信** |
| `--domain ux` × 6 | 表单校验、加载骨架、导航/深链、动效、破坏性操作、焦点/对比度 | 见 §8 逐条引用 | **采信** |
| `--stack vue` | `vue component design tokens consistency` | SFC、路由懒加载（项目已做）、PascalCase | **采信** |

参考清单：`references/quick-reference.md` 全部 10 类 120 条 + `references/pro-rules.md`「Pre-Delivery Checklist」已通读，作为 §10 验收基线。

---

## 1. 执行摘要

**产品定位**：自托管 PT 站点 / 下载器 / 媒体库 / NAS 一体化**运维管理台**（README.md:1–10）。这是典型的 **数据密集型专业工具**，不是消费级 App、不是 AI 产品官网。

**当前观感**：一套完整但方向错配的「iOS 26 液态玻璃」消费级视觉系统（glass.css:1–9）——全屏紫蓝渐变底、半透明磨砂卡、紫色→蓝色渐变主按钮、彩虹渐变导航图标、20–28px 大圆角、spring 回弹、水波纹、彩纸、emoji、Bing 每日壁纸与「星紫/樱粉」主题色。这套语言在 Demo 里惊艳，在 47 个配置/表格页里表现为 **「AI 生成感」：装饰大于信息、层级靠颜色而非结构、每页都在自我表现**。

**目标观感（大厂成熟感）**：1Panel / GitHub / Ant Design Pro / Linear / Shopify Polaris 的共同特征——
中性实色表面 + 1px 边框分层、单一品牌蓝只用于交互态、无装饰性模糊与渐变、8px 网格与 36px 行高、12–14px 可读密度、等宽数字、克制的 120–180ms 状态过渡、组件行为全局一致、可预测（URL 可分享、批量可操作、错误可恢复、键盘可达）。

**关键判断**：项目里**已经存在正确答案**——`extension/theme.css:1–12` 的文件头写的就是「目标：成熟工具型界面——紧凑、克制、信息密度优先；中性深色底，不追逐渐变与辉光；表面扁平、1px 边框分层；单一品牌蓝；8px 栅格；6–10px 圆角；12.5px 基准字号；等宽数字；悬停只做背景微亮」。**建议把扩展端的 token 架构提升为三端唯一真源**，而不是另起炉灶。

**量化基线（本次审计）**：

| 指标 | 现状 | 目标 |
|---|---|---|
| `.vue` 中裸 hex 色值 | 447 处 | 0（组件内） |
| `.css` 中裸 hex 色值 | 422 处 | 仅 token 文件保留 |
| 页面使用 `glass` 类 | 263 行 | 0（改实色 `.surface`） |
| 渐变 `linear/conic-gradient`（.vue） | 47 行 | ≤3（仅图表/进度语义） |
| emoji / confetti 作为图标文案 | 36 行 + i18n 6 处 | 0 |
| 装饰性 `@keyframes` / `animation`（.vue） | 24 行 | 仅加载指示器 |
| 10–11px 字号 | 11 处（CSS）+ 扩展 10.5–11.5px | 0（最小 12px） |
| `el-table` / `el-dialog+drawer` | 250+ / 109 | 统一封装 `PtTable` / `PtDialog` |
| `ElMessage/ElMessageBox` | 464 处 | 统一 `useFeedback` |
| 响应式 `@media`（.vue） | 25 处 | 断点统一 375/768/1024/1440 |
| 分页控件 | 8 处 | 统一 `PtTable` 分页 |
| `aria-*` / 键盘处理 | 40 / 26 处 | 100% 图标按钮有名字 |

**改法策略（低风险）**：**不逐页重写**，而是 ① 新增语义 token 层；② 把现有 `--pt-* / --glass-* / --grad-* / --radius-*` **别名映射**到新 token（一次改动全站换肤）；③ 再用 ratchet（stylelint / CI grep）逐页收紧，防止回潮。已存在的 `scripts/check-no-new-ep.mjs`「棘轮」机制可直接复用。

---

## 2. 范围与页面清单

### 2.1 Web（`frontend/src/router/index.ts`，47 个 view）
概览类：Dashboard、Sites、SiteDetail、Search、Recommend、Subscriptions、EntertainmentOverview、OpsCenter、NasServer、Doctor
内容/站点活动：Announcements、Messages、Notices、SiteRequests、Freeleech、Assessment、Harem、MedalWall、Savept、BrushFlow、Contract、Entertainment
基础设施：Downloaders、Reseed、MediaServer、MediaDetail、FileManager、Transfer、Rss、Trakt、Subtitle、MediaQuality、Cloud、Backup、FilterRules、MediaSettings
自动化：Workflow、WorkflowEditor、Plugins、PluginView、PluginActionDialog、PluginAuditDialog、PluginConfigDialog、PluginLogsDialog、PluginMarketSection
系统与 AI：Settings、GeneralPanel、Assistant、Login

### 2.2 跨端
Flutter：`mobile/lib/features/` 下 25 个页面（dashboard / sites / downloaders / monitor / recommend / savept / medal_wall / entertainment / login / …），主题在 `mobile/lib/theme/glass.dart`。
浏览器扩展：`extension/popup.html`、`options.html`（+ `theme.css`）。

### 2.3 值得保留的现状（不要推倒重来）
- `glass.css` 已有完整 token 阶梯（`--sp-*` / `--fs-*` / `--radius-*` / `--shadow-*` / `--dur-*`）——**结构对，值要换**。
- `glass.css:~660` 已有 `:focus-visible` 与 `@media (prefers-reduced-motion: reduce)` 全局兜底（skill CRITICAL 项已达标）。
- `main.ts` 的 errorOverlay（白屏变可反馈错误）、`glass-reduced` 默认降级（187 处 blur 的性能问题已缓解）、i18n 懒加载。
- `router/index.ts`：路由懒加载、`document.title` 随页面、chunk 失败自动刷新、404 兜底。
- `dialog.css:1–60`：弹窗视口内完整可见、长表单 body 内滚动——成熟度高，保留。
- 导航层 `design/menu-redesign.md` v2：6 分组、收藏/最近、⌘K 命令面板、深链、设置分区化已落地。
- `scripts/check-no-new-ep.mjs`：禁止新页面静态引入 Element Plus 的棘轮——正好用来推行自研组件层。
- `extension/theme.css`：成熟工具型 token 体系，作为新真源。

---

## 3. 「AI 感」九大来源（逐条带证据）

### 3.1 全屏紫蓝渐变 + 半透明磨砂卡片（最大单项）
- `frontend/src/styles/glass.css:210–217`：`body { background: linear-gradient(135deg,#e6f0ff,#f0e6ff) }`，暗色 `#16181f→#1d1b2e`。
- `glass.css:28–32`：`--glass-bg: rgba(255,255,255,.55)`、`--glass-border: rgba(255,255,255,.65)`、紫色投影 `rgba(31,38,135,.12)`。
- `glass.css:236–262`：`.glass` = `backdrop-filter: blur(20px) saturate(180%)` + `::before` 白色内高光「模拟液态玻璃边缘折射」。
- 263 行页面挂 `glass` 类（`Announcements.vue:97`、`Assessment.vue:124,166`、`BrushFlow.vue:270,288`、`Backup.vue:240`、`Contract.vue:177`、`Downloaders.vue:267` …）——**配置页/表格页也磨砂**，文字浮在模糊背景上，是「半成品感」的主因。
- 移动端同款：`mobile/lib/theme/glass.dart:104–117` `GlassBackground` 渐变 + `:180+` `GlassCard` BackdropFilter、radius 20。

> skill 依据：`blur-purpose`（模糊只用于模态背景消除，不做装饰）、`surface-readability`（半透明表面削弱层级）、`effects-match-style`。

### 3.2 彩虹渐变导航图标（最刺眼）
- `frontend/src/layouts/navConfig.ts:23–100`：每个导航项都带 `from/to` 两个 hex，30+ 项各自一套渐变（`#e100ff→#ff6a00`、`#7f00ff→#e100ff`、`#11998e→#38ef7d` …）。
- `frontend/src/layouts/AppSidebar.vue:~80`：`<GlassIcon :name :from :to :size="30">` 把渐变灌进图标。
- `glass.css` 主题色预设再叠 4 套渐变（azure/rose/green/amber，约 790–850 行）。

> 大厂管理台的导航：单色线性图标 + 选中态 2px 左边框/浅色底；颜色**只表示状态，不表示身份**。

### 3.3 渐变按钮 / 大圆角 / pill 徽章
- `glass.css:17–24`：`--grad-primary: linear-gradient(135deg,#6c5ce7,#4285f4)`，另两套危险/成功渐变。
- `glass.css:400–430`：`.pt-btn--primary` 渐变底 + 紫色光晕阴影；`.pt-chip` / `.pt-badge` 一律 `border-radius:999px`。
- `glass.css:38–47`：`--radius-lg:20px / --radius-xl:28px`（大圆角是消费级 App 语言）。

### 3.4 消费级「个性化设置」
- `frontend/src/composables/useUiPrefs.ts:22–31`：5 套主题色「星紫（默认）/苍蓝/樱粉/薄荷绿/落橙」、`uiStyle: glass|modern`、`wallpaperKind`、`mask`、`cardStyle`、`sidebarCollapsed`。
- `useUiPrefs.ts:88–105`：`fetchBingDaily()` 拉必应每日壁纸。
- `frontend/src/components/settings/GeneralPanel.vue:66,78–149`：把上述全部做成了设置项。
- `glass.css:~850`：壁纸 + 遮罩浓度 + 边框卡片风格覆盖。

> 运维控制台不需要「樱粉主题 + 每日壁纸」；这是「AI 玩具感」最强的来源，也带来不可控的对比度（用户上传任意壁纸 + 半透明卡 = 文字不可读）。

### 3.5 装饰性动效泛滥
- `glass.css:560–600`：`pt-spin / pt-pulse / pt-fade-in-up / pt-scale-in`，`--ease-out-spring: cubic-bezier(.34,1.56,.64,1)` 全局回弹。
- `glass.css:600–640`：Element Plus 的 tab 指示条、switch、checkbox、tag 全部被改成「弹性」；`.glass, [class*='glass']` 通配 hover 位移。
- `glass.css:~700–780`：`pt-skeleton` shimmer、`pt-stagger` 列表逐个入场、`pt-num-updated` 数字脉动、`pt-ripple` 水波纹、`pt-link-underline` 中心展开、`pt-pressable` 悬浮微抬。
- `frontend/src/styles/dialog.css:100–125`：注释直接写「水润入场动画」，弹窗 `scale(.94) translateY(12px)` + spring。
- `frontend/src/components/EmptyState.vue:40–50`：空态图标 40px + `es-float 3s infinite` 上下浮动。
- 游戏组件：`CbgSlotGame.vue:115–116` `filter:blur` 无限抖动、`City13DiceGame.vue:119–120` 无限摇晃、`canvas-confetti` 依赖（package.json）。

> skill 依据：`excessive-motion`（每屏 1–2 个关键动效）、`continuous-animation`（无限动画只给加载器）、`motion-meaning`（动效必须表达因果）、`spring-physics` 不适用于后台表格、`exit-faster-than-enter`。

### 3.6 Emoji 当图标 / 文案前缀
- 结构图标：`EmptyState.vue:14` 默认 `📭`；`BigSmallSidebar.vue:44,72` `🏆 🎯` 当分区标题；`DashboardActivityTimeline.vue:33,47` 用 `🎯 / 🎁` 表示事件类型；`SearchFilterBar.vue:135` `🎁` 表示免费；`SpinnerGame.vue:32` 整个奖品环是 emoji 数组。
- 文案：`i18n/locales/zh-CN.ts:268` `🚀 欢迎使用 PTPatronus`、`:853` `✨ 新对话`、`:2750` `暂无待办事项 🎉`、`:2949/:4166/:4279` 等 6 处。
- 共 36 行 UI emoji/confetti + 47 行渐变。

> skill 依据：`no-emoji-icons`（字体依赖、跨平台不一致、无法用 token 控制）、`icon-style-consistent`。

### 3.7 娱乐游戏与主管理台混排
- `navConfig.ts` 把「娱乐 / 勋章墙 / 后宫」与「刷流 / 辅种 / 考核」并列在「站点活动」组。
- 娱乐页 `Entertainment.vue:377,380` 带 `pt-scale-in` 入场；游戏组件普遍 `.hero { background: linear-gradient(...) }` + 56px emoji 图标（`CbgSlotGame.vue:106,114`）、`conic-gradient` 转盘（`CbgWheelGame.vue:29`、`City13WheelGame.vue:34`）。
- 这些页面本身可以保留，但**不应定义主界面的视觉基调**，也不应与运维功能争夺同一层级。

### 3.8 顶栏「AI」徽标与 AI 聊天视觉
- `MainLayout.vue:107–116`：`<router-link class="ai-btn"><GlassIcon name="bot" :size="24"/><span class="ai-label">AI</span></router-link>`——一个写着「AI」的渐变胶囊常驻黄金位置；`MainLayout.vue:285` 该按钮背景是紫→粉渐变。
- `Assistant.vue` 整体玻璃侧栏 + 玻璃聊天气泡（`:171,203`）。
- 「AI 感」既是字面意义的 AI 标签，也是「急于展示 AI 能力」的观感。**功能保留、表达降噪**。

### 3.9 语气与信息密度
- 文案：`glass.css` 注释「水润过渡」「液态玻璃」「呼吸感」「触感」；`dialog.css`「水润入场」。
- 密度失控：`frontend/src/components/downloader/torrentTable.css:95,193` 出现 `font-size:10px`；`filemanager.css:66,261`、`OpsDisk.css:78,195` 11px；扩展 `popup.html` 大量 10.5–11.5px。
- `glass.css:52–58`：`--fs-sm:12px / --fs-body:13px`，正文 13px 偏小；skill 要求正文 ≥16px（移动）/ 后台 ≥14px，12px 只用于密集表格元信息。
- 447+422 处裸 hex 说明：**token 有了，但组件没用**——每页仍在自己发明颜色/圆角/按钮（`radiusHard` 19 处硬编码 `border-radius: Npx`）。
### 3.10 全局外壳 / 登录 / AI 助手的补充实证（D 组逐行审计）
- **对比度实测不达标**：`Assistant.css:94` 渐变气泡白字在 `#4285f4` 上仅 **3.56:1**；`GlobalNoticeBar.vue:103` 红橙渐变白字在橙端仅 **2.19:1**；`AppSidebar.css:55` 分组标题 `opacity:.6` ≈**2.5:1**；扩展 `popup.html:38` `#status` 用 `--ink-3 #636d7c` 在 `#161a21` 上 **3.33:1**（skill `color-accessible-pairs` / `contrast-readability`）。
- **焦点被主动移除**：`CommandPalette.vue:277`、`MainLayout.vue:250`、`Assistant.css:164-167` 出现 `outline:none`；`CommandPalette.vue:184` 把 dialog 误挂 `v-a11y-button` 变成 `role=button`（skill `focus-states` / `keyboard-nav`）。
- **表单基础缺失**：`Login.vue:107-122` 仅 placeholder 无 `<label for>`；`:135` 错误只在表单底部、无 `role=alert`、不可聚焦（skill `input-labels` / `error-summary` / `focus-management`）。
- **键盘/平台细节**：`MainLayout.vue:105` 硬编码 `⌘K`（Windows 用户看到错误提示）；用户菜单是无 tabindex 的 div；`useMainLayout.ts:183` 登出无确认；主题切换只写 localStorage，与 `useUiPrefs.ts` 服务端同步割裂。
- **AI 助手**：`Assistant.vue` 8 处 emoji 作功能图标、会话 ID 不进 URL（刷新/分享丢失）、流式输出时 textarea 被 `disabled` 无法排队输入、12 项配置平铺、删除会话无撤销。
- **娱乐与游戏**：`Entertainment.vue:50-56` 只读 `?site=&game=` 但 `:111` 切换不回写（后退/分享失效）；28 个游戏组件普遍 hero 裸 hex 渐变、emoji 扇区、11px 字、无限抖动/摇晃动画；`BigSmallGame.css:325-335` 7 列表格无粘性表头且行高≈22px；`SpinnerGame.vue:133` 硬等 4100ms 且 reduced-motion 下仍卡 4s。
- **命令面板 / 通知**：`CommandPalette.vue` 用 `🔍 ↗ ↵` 当图标、blur 背景、active 渐变；`NotificationCenter.css:58-63` 双层 blur、`:307` 无限摇铃；`useNotices.ts:92-119` 单条删除/全部已读无确认撤销；删除按钮命中区≈18px。
- **跨端**：`mobile/lib/theme/glass.dart:339-365` 自定义「液态」页面转场未读 `MediaQuery.disableAnimations`；`assistant_page.dart:39` 用 `⚠️`；扩展 `popup.html:178-182` tabbar 全 emoji、5 格里混入「签到」动作、10.5–11.5px 正文、26px 按钮。
### 3.11 仪表盘与运维总览的量化补充（A 组审计）
- **实测对比度全线不达标**：`--text-disabled` 白底 **2.54:1**；`#d97706` **3.19:1**；`#11998e` **3.52:1**；`#2ecc71` **2.10:1**；`#e74c3c` **3.82:1**；`#94a3b8` **2.56:1**（`RealtimeCard.vue:102,134,137`、`SyncCard.vue:149`、`HealthStrip.vue:204`、`device-detail.css:105-107`）。加上 D 组的 3.56 / 2.19 / 3.33 / 2.5:1，**全站至少 10 组颜色组合低于 4.5:1**——这是「看起来廉价」和「不可访问」的共同根因，必须作为 P0 优先于纯视觉改造。
- **主 CTA 泛滥**：`Dashboard.vue:148,174,184,193,276` 同屏 5 个 primary；`Transfer.vue:119-145` 一行 8 个按钮；`NasServer.vue:208-224` 头部 7 个等权按钮；`Sites.vue:161-182` 5 个同权重按钮（skill `primary-action`）。
- **图表普遍不可键盘访问**：`TrendChart.vue:95` / `Sparkline.vue:140-143` 仅 hover 显示数值、`RingChart.vue:53` SVG 无 `role`/`aria`；skill `--domain chart` 要求可见数据表兜底 + 键盘可聚焦读数。
- **「一键修复」零防护**：`Doctor.vue:28-32,69` 直接执行修复，无确认、无 dry-run、无快照/回滚，失败仅 toast——高风险交互，必须补确认 + 可撤销。
- **多个状态只靠颜色**：`Doctor.vue:84,126-131` 的 `.dot`、`NasServer` 状态色、`HealthStrip` 健康色（skill `color-not-only`）。



---

## 4. 目标设计系统（可直接落地的 token）

### 4.1 风格定调
**Minimalism & Swiss + data-dense-dashboard**（skill 双次验证）：实色表面、1px 边框、中性灰阶、单一交互蓝、12 栏网格、8px 间距、36px 行高、240px 侧栏、56px 头栏、tabular-nums、无装饰模糊/渐变、悬停只做背景/边框微变。

参照：`extension/theme.css:1–45`（已经是这套语言）。**三端共用同一组语义 token 名**。

### 4.2 新增 `frontend/src/styles/tokens.css`（语义层，唯一真源）

~~~css
:root {
  /* 表面 / 文字（浅色） */
  --color-bg: #f8fafc;
  --color-surface: #ffffff;
  --color-surface-2: #f1f5f9;
  --color-surface-hover: #f1f5f9;
  --color-border: #e2e8f0;
  --color-border-strong: #cbd5e1;
  --color-text: #1e293b;          /* 14.7:1 on #fff */
  --color-text-secondary: #475569;/* 7.5:1  */
  --color-text-muted: #64748b;    /* 4.8:1  */
  --color-text-inverse: #ffffff;

  /* 交互与语义 */
  --color-primary: #2563eb;
  --color-primary-hover: #1d4ed8;
  --color-primary-soft: #eff6ff;
  --color-success: #15803d;  --color-success-soft: #f0fdf4;
  --color-warning: #b45309;  --color-warning-soft: #fffbeb;
  --color-danger:  #dc2626;  --color-danger-soft:  #fef2f2;
  --color-focus: #2563eb;

  /* 几何：8px 网格 / 克制圆角 */
  --space-1: 4px; --space-2: 8px; --space-3: 12px;
  --space-4: 16px; --space-5: 20px; --space-6: 24px; --space-8: 32px;
  --radius-control: 6px; --radius-card: 8px; --radius-panel: 10px;
  --radius-pill: 999px; /* 仅状态点，不用于徽章/按钮 */
  --shadow-card: 0 1px 2px rgba(15,23,42,.04);
  --shadow-popover: 0 8px 24px rgba(15,23,42,.12);

  /* 排版：中文优先系统字体 */
  --font-sans: -apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC',
               'Microsoft YaHei', 'Noto Sans CJK SC', Inter, sans-serif;
  --font-mono: ui-monospace, 'SF Mono', 'Cascadia Code', Consolas, monospace;
  --fs-xs: 12px; --fs-sm: 13px; --fs-md: 14px; --fs-lg: 16px;
  --fs-xl: 20px; --fs-2xl: 24px;
  --line-height: 1.5;

  /* 布局基准（data-dense-dashboard 验证值） */
  --grid-gap: 8px; --card-padding: 12px;
  --table-row-height: 36px; --sidebar-width: 240px; --header-height: 56px;

  /* 动效：无 spring、无回弹、退场快于入场 */
  --dur-fast: 120ms; --dur-base: 180ms; --dur-slow: 240ms;
  --ease: cubic-bezier(.2,.8,.2,1);
}

:root.dark {
  --color-bg: #0f1115;
  --color-surface: #161a21;
  --color-surface-2: #1c222c;
  --color-surface-hover: #1f2530;
  --color-border: #262c36;
  --color-border-strong: #333b48;
  --color-text: #e6e9ef;
  --color-text-secondary: #9aa4b2;
  --color-text-muted: #7d8797;   /* 复核 ≥4.5:1 on #161a21 */
  --color-primary: #4c8dff;
  --color-primary-hover: #6ea1ff;
  --color-primary-soft: rgba(76,141,255,.14);
  --color-success: #3fb950; --color-warning: #d29922; --color-danger: #f85149;
  --shadow-card: 0 1px 2px rgba(0,0,0,.3);
  --shadow-popover: 0 8px 24px rgba(0,0,0,.5);
}
~~~

### 4.3 兼容映射层（**一次改动全站换肤，零逐页重写**）
在 `glass.css` 顶部 `@import './tokens.css'`，然后把旧 token 全部别名过去：

~~~css
:root {
  --pt-primary: var(--color-primary);
  --pt-primary-2: var(--color-primary-hover);
  --pt-success: var(--color-success);
  --pt-danger: var(--color-danger);
  --pt-warn: var(--color-warning);
  --grad-primary: var(--color-primary);  /* 渐变按钮自动变实色 */
  --grad-danger: var(--color-danger);
  --grad-success: var(--color-success);
  --glass-bg: var(--color-surface);
  --glass-bg-strong: var(--color-surface);
  --glass-border: var(--color-border);
  --glass-shadow: var(--shadow-card);
  --surface-1: var(--color-surface);
  --surface-2: var(--color-surface-2);
  --surface-border: var(--color-border);
  --surface-hover: var(--color-primary-soft);
  --divider: var(--color-border);
  --text-strong: var(--color-text);
  --text-muted: var(--color-text-secondary);
  --text-disabled: var(--color-text-muted);
  --radius-xs: var(--radius-control); --radius-sm: var(--radius-control);
  --radius-md: var(--radius-card); --radius-lg: var(--radius-card);
  --radius-xl: var(--radius-panel); --radius-pill: 999px;
  --fs-2xs: var(--fs-xs); --fs-xs: var(--fs-xs); --fs-sm: var(--fs-sm);
  --fs-body: var(--fs-md); --fs-md: var(--fs-md); --fs-lg: var(--fs-lg);
  --fs-xl: var(--fs-xl); --fs-title: var(--fs-2xl); --fs-hero: 30px;
  --ease-out-spring: var(--ease); --ease-out-smooth: var(--ease);
  --dur-base: 180ms; --dur-slow: 240ms; --dur-fast: 120ms;
  --blur: 0px;
}
~~~

同时改 `glass.css` 的渲染规则（保类名、换实现，页面 template 不动）：
- `body`：`background: var(--color-bg);`（删除 210–217 行渐变与 850 行后壁纸分支）。
- `.glass, .glass-bar`：`background: var(--color-surface); border: 1px solid var(--color-border); border-radius: var(--radius-card); box-shadow: var(--shadow-card); backdrop-filter: none;`；删除 `.glass::before` rim 高光（`blur-purpose`）。
- `.solid-card` / `.glass` 合并为同一实色卡；`.glass-hover:hover` 只改 `border-color: var(--color-border-strong)`，删除 `translateY(-2px)`。
- `.pt-btn--primary`：实色 `var(--color-primary)`、`box-shadow:none`；`:active` 只做 `background: var(--color-primary-hover)`。
- `.pt-chip / .pt-badge`：`border-radius: var(--radius-control)`；状态用 soft 底 + 语义色文字 + **图标或文字**（`color-not-only`）。
- 全局 `[class*='glass']` 通配 transition 删除；`pt-ripple / pt-stagger / pt-num-updated / pt-pressable / pt-link-underline` 标记废弃（保留类但置空），下个版本删除。
- `:focus-visible`（已有）改为 `outline: 2px solid var(--color-focus); outline-offset: 2px`（skill `focus-appearance`），并加 `scroll-padding-top: var(--header-height)`（`focus-not-obscured`）。

### 4.4 组件层（配合 `check-no-new-ep.mjs` 棘轮）
新增 `frontend/src/components/ui/`：`PtButton`、`PtCard`、`PtTable`（粘性表头/排序/URL 状态/批量栏/分页/骨架/空态）、`PtTag`、`PtDialog`（沿用 dialog.css 的视口约束）、`PtFormField`（label + helper + 行内错误 + aria-describedby）、`PtEmptyState`（SVG 图标，无浮动）、`PtSkeleton`（固定高度防 CLS）、`useFeedback`（统一 toast / confirm / undo，aria-live polite）。
规则：**新页面/重构页禁止直接写 `el-*` 与裸 hex**；CI 用 `check-no-new-ep.mjs` 同款棘轮 + stylelint 校验。

---

## 5. 详细修改方案（按优先级）

### P0 · 换基调（2–3 天，收益最大、风险最低）
| # | 改动 | 落点 | 现状 → 目标 |
|---|---|---|---|
| P0-1 | 新 token + 兼容映射 | 新增 `tokens.css`；`glass.css:1–200` | 紫蓝渐变 → 中性 slate + 信任蓝；`--grad-*` 全部指向实色 |
| P0-2 | 去掉全屏渐变与壁纸 | `glass.css:210–217, ~850`；`useUiPrefs.ts:88–105` | 删除 body 渐变、Bing 壁纸、遮罩浓度、卡片风格设置；保留「浅/深/跟随系统 + 密度」 |
| P0-3 | 玻璃卡变实色卡 | `glass.css:236–262` | 关 backdrop-filter、删 rim 高光、删 hover 位移；263 处 `glass` 类名不变 |
| P0-4 | 导航图标去彩虹 | `navConfig.ts:23–100` + `AppSidebar.vue:~80` + `GlassIcon.vue` | 删除 `NavItem.from/to` 字段；图标换单色 20px 线性图标（Element Plus icons 或 Lucide，统一 1.75px 描边）；选中态=2px 左边框 + `--color-primary-soft` 底 + 主色文字 |
| P0-5 | 主题色预设收敛 | `useUiPrefs.ts:33–40`、`GeneralPanel.vue:66` | 5 套「星紫/樱粉…」→ 默认蓝 + 石墨灰 2 套（或移除）；旧值自动迁移，不丢用户数据 |
| P0-6 | 顶栏 AI 徽标降噪 | `MainLayout.vue:107–116,285` | 「AI」渐变胶囊 → 普通图标按钮 `助手`（`aria-label="助手"`），移入「更多」或保留但无渐变/无文字标签；功能与路由不变 |
| P0-7 | 装饰动效清零 | `glass.css:560–780`、`dialog.css:100–125`、`EmptyState.vue:40–50` | spring → 标准 ease；弹窗淡入 120ms、退场 80ms；删水波纹/数字脉动/列表 stagger/链接下划线展开/空态浮动；保留加载 spinner |
| P0-8 | Emoji 清零 | 36 行 UI + `zh-CN.ts:268,853,2750,2949,4166,4279` 与 en-US 对应项 | 全部换 SVG 图标或纯文字；`EmptyState` 默认图标改 `Inbox` 线性图标 24px |
| P0-9 | 对比度与焦点急救 | `Assistant.css:94`、`GlobalNoticeBar.vue:103`、`AppSidebar.css:55`、`extension/popup.html:38`、`CommandPalette.vue:277,184`、`MainLayout.vue:250` | 4 组低对比度组合改实色语义 token ≥4.5:1；删除 `outline:none`；dialog 去掉误挂的 `v-a11y-button`、补焦点陷阱与归还焦点 |
| P0-10 | 破坏性操作统一 | `useMainLayout.ts:106,183`、`useNotices.ts:92-119`、`Sites.vue:198-199`、`Backup.vue:114`、`EventBet.vue:145`、`Assistant.vue:196` | 新建 `useConfirmUndo()`：写明对象与后果的确认 + 5–8s 撤销 toast；清空收藏、登出、删除站点/会话/通知、导入覆盖、下注全部接入 |

> **兼容开关**：`body[data-ui-style='classic']` 保留旧玻璃实现 1–2 个版本，给老用户过渡；默认新用户/新安装走 modern。已有 `data-ui-style='modern'`（`glass.css:~830`）可直接改名复用。

### P1 · 组件与一致性（1 周）
1. **颜色收敛**：447 + 422 处裸 hex 用脚本批量替换为 `var(--color-*)`；`tokens.css` 之外禁止 hex，stylelint 新增规则 `color-no-hex`（ignore: tokens.css / theme.css）。
2. **圆角收敛**：`--radius-lg/xl` 统一 8/10px；19 处硬编码 `border-radius:Npx` → token；pill 仅用于状态点。
3. **字号收敛**：`--fs-body:14px`；10/11px 全部提到 12px（`torrentTable.css:95,193`、`filemanager.css:66,261`、`OpsDisk.css:78,195`）；`line-height:1.5`；数字列 `font-variant-numeric: tabular-nums`（已有，保留）。
4. **按钮/标签统一**：`pt-btn` / `el-button` / 各页手写按钮 → `PtButton`；每屏**仅 1 个主按钮**（`primary-action`），其余为默认/文字按钮；危险按钮与主要操作视觉分离（`destructive-emphasis`）。
5. **弹窗统一**：109 处 `el-dialog/el-drawer` 走 `PtDialog`，保留 `dialog.css` 的视口约束；补 Esc 关闭、焦点陷阱、关闭未保存确认（`sheet-dismiss-confirm`）、`aria-modal`。
6. **图标系统**：单一图标族 + 16/20/24px 三档 token；装饰图标 `aria-hidden="true"`，图标按钮 `:aria-label`（目前仅 40 处 a11y 属性 / 464 次消息调用），交互图标暴露 `aria-pressed/expanded`（`icon-context-accessibility`）。
7. **暗色对比复核**：对全部 token 前后景组合跑 4.5:1 校验（`color-accessible-pairs`），重点修 `--text-muted`、soft tint 上的语义色文字；`glass.css:150–180` 暗色变量改为**去饱和提亮**而非简单反色（`color-dark-mode`）。
8. **娱乐内容隔离**：`navConfig.ts` 把「娱乐 / 勋章墙 / 后宫」收进可折叠「娱乐（可选）」组（或 `showEntertainment` 特性开关，新安装默认关）；游戏组件内 emoji/conic-gradient/confetti 收敛为「仅娱乐页、且 `prefers-reduced-motion` 下禁用」，字体 ≥12px。

### P2 · 交互逻辑便捷化（1–2 周，详见 §8）
表格、表单、异步反馈、导航状态、批量/撤销、通知、图表全面按 skill `ux` 规则补齐。

### P3 · 跨端与门禁（1 周，详见 §9/§10）
Flutter 与扩展同步 token；CI 加视觉/无障碍门禁；E2E 回归。

---

## 6. 逐页修改清单（47 个 view）

> 下表为按页面类型的**基线动作**；四位并行审计（A 仪表盘 / B 列表表格 / C 配置表单 / D 外壳与 AI 热点）返回的逐页证据与特化建议见 `§6.1`（交付时补齐）。

| 页面 | 类别 | 主要改动 |
|---|---|---|
| Dashboard | 概览 | KPI 实色卡、8px gap/12px padding、渐变指标改数字+单位、Sparkline 加直接标签与数据表兜底、任务卡片去重、去掉 stagger 入场 |
| Sites / SiteDetail | 列表+详情 | 站点卡改紧凑表格/统一卡；筛选与排序进 URL；面包屑（3 层以上）；详情页 Tab 状态进 query；粘性表头、36px 行高 |
| Search | 搜索 | 筛选栏常驻+可折叠、`🎁` 免费标签改语义 Tag+图标、结果骨架固定高度防 CLS、批量推送、空态带引导 |
| Recommend / MediaDetail | 内容 | 海报 `width/height` 或 `aspect-ratio` 防跳动；评分/质量用语义色+文字；去渐变头图 |
| Subscriptions / Rss | 订阅 | RSS 并入订阅 Tab（已 redirect 的直接收敛）、批量启停、规则表单行内校验 |
| Transfer / Downloaders / Reseed | 任务表格 | `PtTable` 统一（36px 行 / 粘性头 / 批量 / 分页 / URL 筛选）、进度条可访问、错误可重试、`.torrent` 拖拽改「选择文件」按钮（`dragging-alternative`） |
| FileManager / Cloud | 文件 | 面包屑、列宽持久化、批量操作栏、拖拽上传提供按钮替代、路径进 URL |
| MediaServer / NasServer / OpsCenter / Doctor | 监控运维 | 等宽数字、状态色+文字（`color-not-only`）、图表加可聚焦数据表、Tab/时间范围进 URL、终端页保留等宽但字号 ≥12px |
| Workflow / WorkflowEditor | 自动化 | 节点卡 token 化；画布缩放/删除提供键盘与按钮替代；节点参数表单行内校验 + 错误摘要；运行态骨架 |
| Plugins / PluginView / 5 个 Plugin*Dialog | 插件 | 实色卡与表格化；配置表单 label+helper+就近错误；日志等宽、虚拟滚动；卸载/删除二次确认 + 撤销 |
| Settings / GeneralPanel | 设置 | 删个性化（P0-2/P0-5）；分区导航 + 深链（已有 `?section=&tab=` 扩展）；保存态 loading→成功/失败；危险操作确认 |
| Backup / FilterRules / MediaSettings / MediaQuality / MediaServer / Trakt / Subtitle / Cloud | 配置 | 统一 `PtFormField`；开关即时保存反馈；必填标记；`aria-describedby` 关联错误 |
| BrushFlow / Contract / Assessment / Savept / Harem / MedalWall / Freeleech / Announcements / Notices / Messages / SiteRequests | 站点活动 | 去彩卡/渐变头、表格化 + 批量、状态语义化、空态统一、未读批量已读 |
| Entertainment / EntertainmentOverview | 娱乐 | 隔离分组；去 emoji/confetti/无限动画；字号 ≥12px；可关闭 |
| Login | 登录 | 去渐变背景与玻璃卡；品牌克制（logo+产品名+版本）；密码显隐切换、`autocomplete` 支持密码管理器（`accessible-authentication`）、就近错误、加载态 |
| Assistant | AI 助手 | 去「AI」品牌强调与渐变气泡；中性消息气泡、等宽代码块、复制/重试/停止生成、`Shift+Enter` 换行说明、草稿自动保存、错误可恢复；入口按 P0-6 降噪 |

### 6.1 逐页审计证据（并行子代理结果汇总）

> 四组并行只读审计，按 **D 全局外壳与跨端 → A 仪表盘与总览 → B 列表/表格/内容 → C 配置/表单/流程** 排列；每组均含「页面清单 / 逐页现状→改法 / 共性优先级 / 已具备勿重复」四部分。A、D 两组以子代理「定稿」版为准（内容与其首版一致，仅更紧凑）。所有证据为 `文件:行号`，路径省略前缀 `frontend/src/`。
>
> **A 组定稿校注（补充行号）**：`.health-item b` → `HealthStrip.vue:186`、`.sync-percent` → `SyncCard.vue:91`；`.f-rec{opacity:.7}` → `Doctor.vue:146`；`QuickTools.vue` 复制按钮 → `:153,172,175`。

#### D 组 · 全局外壳 / 登录 / AI 助手 / 娱乐 / 跨端（已完成）

**页面与风格证据**

| 文件 | 用途 | 视觉风格证据（文件:行号） |
|---|---|---|
| layouts/MainLayout.vue + useMainLayout.ts | 全局外壳/顶栏 | `.glass-bar` :87、移动端 `backdrop-filter:blur(4px)` :306、AI 激活渐变 :285、硬编码 ⌘K :105 |
| layouts/AppSidebar.vue/.css + navConfig.ts | 侧栏导航 | `.glass` AppSidebar.vue:51、激活渐变 css:125、无限摇铃/涟漪 css:210/:217、26 组裸 hex 渐变 navConfig.ts:26-89 |
| views/Login.vue/.css | 登录 | glass :82、渐变提交 css:105、「水润动效」入场/位移 css:191-230 |
| views/Assistant.vue/.css | AI 助手 | 🔒⚙🗑🧠🔧💬 :165/176/196/198/206/217/227、渐变气泡 css:94、气泡入场 css:247 |
| views/Entertainment.vue + 28 个游戏组件 | 娱乐分发 | 🎮 :257、`pt-scale-in` :377、切屏强制重挂载 :144、hero 裸 hex 渐变（BetGame.vue:202 / EventBet.css:11 / SpinnerGame.css:11）、emoji 扇区 SpinnerGame.vue:32/:207、11px 字 CbgWheelGame.vue:138、无限动画 City13DiceGame.vue:119 |
| components/CommandPalette.vue | ⌘K 面板 | 🔍 :186、↗ :217、↵ :226、blur :249、active 渐变 :313、`outline:none` :277 |
| NotificationCenter.vue/.css | 通知铃铛 | 双层 blur css:58-63、无限摇铃 css:307、内联裸 hex :236/:244 |
| GlobalNoticeBar.vue | 全局告警条 | 红橙渐变 :103（白字橙端 **2.19:1**）、3px 按钮 :174 |
| TaskIndicator.vue / WsStatusDot.vue | 任务/WS 状态 | 3px 进度条 :72、无限 pulse WsStatusDot:54；tabular-nums :67（正面） |
| SptIcon.vue / GlassIcon.vue | 图标系统 | GlassIcon.vue:9-17 / :200 渐变 / :216-233 rim；SptIcon.vue:7 Lucide SVG（正面，可作为统一图标族基座） |
| styles/glass.css + useUiPrefs.ts | token/偏好 | body 渐变 :213、blur :259、rim :270、shimmer/ripple/stagger :725/:785/:749、默认 `uiStyle:'glass'` useUiPrefs.ts:25、Bing 壁纸 :92 |
| mobile/lib/theme/glass.dart + 3 代表页 | Flutter | 渐变背景 :110-116、blur :253、缩放 :302、转场 :339-365 未读 `MediaQuery.disableAnimations`、PTFs.body=13 :37 |
| extension/popup.html/.js + theme.css | 扩展弹窗 | emoji tabbar :178-182 / ⚙ :130 / ⬇ popup.js:273、10.5–11.5px :33/:38/:67/:68/:91/:113 |

**视觉去 AI 感（现状 → 建议）**
- MainLayout.vue:87/:306/:285 磨砂 + AI 激活渐变 → `.solid-card`（glass.css:288）+ `--surface-1`，激活改 2px 主色底边；:117 GlassIcon → `<SptIcon name="bot">` currentColor。
- navConfig.ts:26-89、useMainLayout.ts:141-163 每项裸 hex 渐变 → 删 `from/to`，图标统一 `--text-secondary` 单色 Lucide；AppSidebar.css:125 激活渐变 → `--pt-primary-bg` + 2px 左条。
- Login.vue:82 glass、Login.css:105 渐变按钮、css:191-230 入场/位移/缩放 → 实色卡 + 单色主按钮，删 `pt-scale-in` / `translateY` / `brightness`。
- Assistant.vue 8 处 emoji、Assistant.css:94 渐变气泡（白字在 `#4285f4` 仅 **3.56:1**）、:247 气泡入场 → SptIcon + `--pt-primary` 实色 + 静态。
- Entertainment.vue:257 🎮、:377-380 入场、28 个游戏组件 hero 裸 hex 渐变与 emoji 扇区 → SVG 空态、去动画、`--surface-2` 实色 + 1px 边框；BigSmallGame.vue:212/258/320/368、SimpleGameCard.vue:104/120/127、SpinnerGame.vue:213/240/247 数据面板套 `.glass` → `.solid-card`。
- CommandPalette.vue:186/:217/:226 符号 → SptIcon；:249/:254 blur → 实色；:313 active 渐变 → `--pt-primary-bg` + 左条。GlobalNoticeBar.vue:103 红橙渐变（**2.19:1**）→ `--pt-danger` 实色或浅底深字。NotificationCenter.css:61-63 双层 blur → 实色；NotificationCenter.vue:236/:244 内联裸 hex → `--pt-*-bg`。
- glass.css:213/259-279/725-811 渐变、blur、rim、ripple/stagger/num-pulse → **保留** token(:14-145)、solid-card(:288)、focus-visible(:711)、reduced-motion(:659-677)、tabular-nums(:211)、skeleton(:725)，删装饰类；`.pt-btn--primary`:408 渐变 → 实色。
- 移动端 glass.dart:110-116/:253/:302/:339-365 → 实色背景、跳过 BackdropFilter、去缩放、平台标准转场并短路 `disableAnimations`；娱乐页 emoji 与渐变同步清理。

**交互便捷性（现状 → 建议）**
- MainLayout.vue:105 硬编码 ⌘K → 按平台显示 `Ctrl/⌘ K`；:120-142 用户菜单为无 tabindex 的 div 且 :250 `outline:none` → `<el-button text>`；useMainLayout.ts:171-174 主题只写 localStorage 与 useUiPrefs.ts:7 服务端同步割裂 → 改 `useUiPrefs.set`；:183 logout 无确认 → `ElMessageBox.confirm`。
- AppSidebar.css:146-153 收藏星标仅 hover 显示 → `:focus-within` 常显；useMainLayout.ts:106-109 清空收藏无确认/撤销 → 确认 + 撤销 toast。
- Login.vue:107-122 只有 placeholder → 可见 `<label for>` + `aria-describedby`；:135 错误在表单下方且无 `role=alert` → 就近行内错误 + 顶部 `tabindex="-1"` 可聚焦摘要；:136-141 demo/OIDC 与主按钮并列 → 每屏 1 主 CTA，次要改 link 置分隔线下（:128 loading 保留）。
- Assistant.vue:196 删除有确认但无撤销 → 5s 撤销；:249-256 流式时 textarea `disabled` → 可继续输入并排队；:266 弹窗固定 560px → `min(560px,92vw)`；:267-373 12 项配置平铺 → `el-tabs` 分组；会话/激活 ID 不进 URL → `router.replace({query:{c:activeId}})` 支持深链刷新恢复。
- Entertainment.vue:50-56 只读 `?site=&game=` 但 :111 切换不回写 → `watch` 内 `router.replace` + 从 `route.query` 初始化；:144 `v-loading` → `el-skeleton` 且切游戏保状态；缺跨站批量 → 常用玩法快捷栏；EventBet.vue:145-147 下注无二次确认 → `ElMessageBox.confirm` + 金额就近校验；:70-72 错误走远端 toast → 行内错误。
- CommandPalette.vue:184 dialog 误挂 `v-a11y-button`（变 `role=button`）→ 删指令 + 焦点陷阱 + 归还焦点 + `aria-activedescendant`；:277 `outline:none` 干掉焦点环 → 删该行让 glass.css:711 生效。NotificationCenter.vue:65-70 单条删除/全部已读无确认撤销 → 补（:73 清空已有确认，正面）。
- 娱乐页 BigSmallGame.css:325-335 7 列 grid 无 sticky、行高≈22px → `overflow-x:auto` + `.rec-head{position:sticky;top:0;background:var(--surface-1)}` + 36px 行高 + tabular-nums；SpinnerGame.vue:133 硬等 4100ms（reduced-motion 仍卡 4s）→ `matchMedia` 短路。
- 扩展 popup.html:177-183 5 格 tabbar 混入「签到」动作 → 4 导航 + 1 独立 CTA；assistant_page.dart:82-93 删除确认补撤销；entertainment_page.dart:63 泛化 toast 无重试 → 空态重试（dashboard_page.dart:164-169 可作范例）。

**一致性与可访问性（现状 → 建议）**
- 对比度：`#fff/#4285f4` 3.56:1、`#fff/#f39c12` 2.19:1、`--ink-3 #636d7c / #161a21` 3.33:1（popup.html:38 `#status`）、AppSidebar.css:55 分组标题 `opacity:.6` ≈2.5:1 → 全部改实色语义 token 并复核 ≥4.5:1；popup.html:38 改 `--ink-2` + `role="status" aria-live="polite"`。
- 焦点/键盘：CommandPalette.vue:277/:184、Login.vue:135、MainLayout.vue:250、Assistant.css:164-167 `outline:none`（:257-259 有 box-shadow 环，保留并改 `:focus-visible`）→ 修复后做全键盘走查。
- 命中区：NotificationCenter.vue:288 删除按钮 2px padding≈18px、GlobalNoticeBar.vue:174 3px 按钮≈22px、popup.html:23 26px、SpinnerGame.css:121 9px 内边距 → ≥24px（移动 44px）；AppSidebar.css:90-97 12px/0.55 操作 → 实色 + 24px 命中区。
- 字号：CbgWheelGame.vue:138、TangptLotteryGame.vue:149、FishingGame.vue:219、FriendTradeGame.vue:232、SimpleGameCard.vue:157 11px → ≥12px（移动 ≥16px）；glass.css:80-87 正文 13px 且 MainLayout.vue:294-335 移动端未放大 → `@media(max-width:720px)` 提到 ≥16px；mobile glass.dart:37 body=13 → 16；dashboard_page.dart:127 窄屏 `toolbarHeight:40` → 44。
- 无限动画：AppSidebar.css:210/:217、NotificationCenter.css:307、WsStatusDot.vue:54、City13DiceGame.vue:119、CbgSlotGame.vue:115、LongPTDailyGame.vue:251 → 静态状态点/有限次动画。
- 破坏性操作（P0）：useMainLayout.ts:106、useNotices.ts:92-119、GlobalNoticeBar.vue:56、EventBet.vue:145 → 统一 `useConfirmUndo()`（确认 + 撤销 toast）。
- 平台文案：Login.css:144 内网示例 IP → 中性文案；MainLayout.vue:105 键位按平台显示；中英混排统一。

**已具备、勿重复造轮子**：glass.css token / solid-card / focus-visible / reduced-motion / tabular-nums、`a11yButton.ts`、NotificationCenter.vue:73 清空确认、`useAgentChat.ts:85` 删除确认、`extension/theme.css:59-67`、TaskIndicator.vue:67、SptIcon.vue（Lucide SVG 基座）、JggGameUi.vue:92-103 与 SpinnerGame.vue:217-219 的连抽、`dashboard_page.dart:164-169` 空态重试。

**D 组优先级**：
- **P0** ① 全端 emoji 当图标（游戏组件 grep ≥127 处、popup.html:178-182、EmptyState.vue:18）→ 统一 SptIcon/Lucide 并删字面量；② 对比度不合格（上述 4 组）→ 实色 + 语义 token 全 ≥4.5:1；③ 焦点/键盘（CommandPalette.vue:277/:184、Login.vue:135、MainLayout.vue:250）→ 修复后键盘走查；④ 破坏性操作统一 `useConfirmUndo()`。
- **P1** ⑤ 玻璃/渐变/无限动画（AI 感主因）→ 默认 `uiStyle:'modern'`、实色卡、删装饰动画，移动端同步；⑥ 字号与命中区 → 8px 网格、≥24/44px、移动 ≥16px；⑦ 深链/批量（Entertainment.vue:50-56 只读不写、Assistant 会话无 URL）→ URL 状态 + 批量栏；⑧ 数据密集页 `.glass` 包表 → solid-card + 粘性表头 + 36px 行高。
- **P2** ⑨ 裸 hex（navConfig.ts:26-89、useMainLayout.ts:141-163、useUiPrefs.ts:34-38、游戏调色板）→ 语义 token；⑩ 骨架覆盖不足（多数仅 `v-loading`；仅 glass.css:725、popup.js:336、dashboard_page.dart:161 有）→ 推广 `el-skeleton` / `PTLoadingView`，修弹窗宽度与 reduced-motion 下 4s 空等。

**D 组定稿补充（以子代理「定稿」版为准，覆盖前两版）**
- `CommandPalette.vue:198-207` listbox 无 `aria-activedescendant` / 选项 id → 补；`:236` 与 `MainLayout.vue:105` 键位显示统一。
- `NotificationCenter.css:45` 紧急徽标 `#e74c3c` 白字 **3.82:1** → 提亮或改深底；`:163 opacity:.82` → 实色；`:174` 12px 操作 → 24px 命中区；`GlobalNoticeBar.vue:65` body 用 `v-a11y-button` 且无 `aria-expanded` → `<button aria-expanded>`；`:56` dismiss 无撤销 → 补。
- `useUiPrefs.ts:34-38` `ACCENT_PRESETS` 裸 hex → 语义 token；`glass.css:659-677` 的 reduced-motion 是 CSS 兜底，**管不到 JS 动画**（`SpinnerGame.vue:133` 4.1s 硬等）→ 组件内补 `matchMedia('(prefers-reduced-motion: reduce)')` 短路。
- `dialog.css:90-125` 的 `!important` 优先级高于全局 reduced-motion（`glass.css:659-665`）→ 在 dialog.css 内单独补 `@media (prefers-reduced-motion:reduce)`。
- 正面项补充：`popup.js:336` 骨架、`dashboard_page.dart:161` 加载视图、`theme.css:59-67` focus/reduced-motion、`glass.css:904-913` 已有的 `data-ui-style='modern'` 实现（可直接改默认值，无需新写）。

#### A 组 · 仪表盘与总览（已完成）

> **全局前提（避免误判）**：已存在 `:focus-visible` 焦点环(`glass.css:711`)、`prefers-reduced-motion` 兜底(`glass.css:659`)、`.solid-card/--surface-1` 实色卡(`glass.css:288`)、`--fs-2xs=12px/--fs-body=13px`(`glass.css:80-88,124`)。问题在于本组文件**绕开它们**。

**页面清单**

| 文件 | 用途 | 视觉风格证据 |
|---|---|---|
| Dashboard.vue / Dashboard.css | PT 站点总览首页 | staggered 入场 `Dashboard.css:245-268`、渐变圆 `:220`、🧭 `Dashboard.vue:274`、GlassIcon 裸 hex `:145,:149` |
| dashboard/ 指标·图表·状态卡（HealthStrip、SyncCard、TrendSection、TrendChart、Sparkline、RingChart、AICard、AutomationCard、ActivityTimeline、RealtimeCard、ResourceCard、TaskCards） | 健康条/同步/趋势/自动化/AI/时间线/资源/待办 | glass `HealthStrip.vue:69,80` / `SyncCard.vue:20`；emoji `ActivityTimeline.vue:30-67,89`、`HealthStrip.vue:81`、`TrendSection.vue:102`；面积渐变 `Sparkline.vue:66-68`；裸 hex `TrendSection.css:54-70`、`AutomationCard.vue:64-67`；磨砂 tooltip `TrendChart.vue:194` |
| dashboard/ 工作台·工具·弹窗（Workbench、Widgets、QuickTools、ExportDialog） | 天序工作台/快捷工具/部件定制/导出 | glass `Workbench.vue:138` / `QuickTools.vue:142` / `Widgets.vue:116`；emoji `Workbench.vue:78-83,208,272`、`QuickTools.vue:147,163,184,202`；`transition:all` `DashboardWorkbench.css:70` |
| EntertainmentOverview.vue | 娱乐游戏矩阵总览 | glass `:64-89`、emoji `:21-26,60,77,84,103`、hover 位移 `:126-127`、裸 hex `:135-138` |
| OpsCenter.vue | 8 合 1 运维中心 | 裸 hex `:133,145`；内联 SVG `:73` 为正例 |
| ops/ Ops*.css（Disk/Monitor/Process/Ssh/Terminal） | 运维面板 | 渐变 `OpsDisk.css:116,221`、发光 `OpsMonitor.css:200-201`、表头 blur `OpsProcess.css:103`、hover 上浮 `OpsTerminal.css:71`、11px `OpsDisk.css:78,195` |
| NasServer.vue/.css + monitor/device-detail.css | NAS 设备/监控/部署 | glass-bar `:203`、glass `:228,306`、⚠/📦 `:229,307`；磨砂卡 `device-detail.css:91-92`、渐变 hero `:107`、裸 hex `:48-66` |
| Doctor.vue | 系统体检 + 一键修复 | glass `:82`、裸 hex `:19`、staggered `:158-178`、hover 位移 `:182-184` |

**逐页「现状 → 改法」**

| 页面 | 去 AI 感 | 交互便捷 | 一致性 / 可访问性 |
|---|---|---|---|
| Dashboard 主页 | 卡片「水润」staggered 6 段+延迟(`Dashboard.css:245-268`)、ob-num 渐变圆(`:220`)、GlassIcon 双色渐变+裸 hex(`Dashboard.vue:145,149`、`SyncCard.vue:25`) → 删 `.zone` 动画、渐变改 `var(--pt-primary)`、GlassIcon 换 `<el-icon>` 复用 `OpsCenter.vue:73` 内联 SVG（package.json 无 lucide 依赖）；🧭 `:274` 换 SVG | 同屏 **5 个 primary CTA**(`Dashboard.vue:148,174,184,193,276`) → 仅留 `:148` 刷新为主 CTA；周期 7/30/90(`:132-143`)与指标切换(`TrendSection.vue:38-51`)只存模块 ref(`useDashboardData.ts:16-17`)，刷新/分享丢状态 → `router.replace({query:{days,mode}})` + `onMounted` 读 query（正例 `HealthStrip.vue:22-24` 已用 `?matrix=`）；重置配置无确认/撤销(`ExportDialog.vue:62-70,239`) → 确认 + 快照可撤销 | `.period-control button` padding 4px 10px(`Dashboard.css:50-56`)≈22px<24px → `min-height:24px;padding:6px 12px`；正文 13px 移动端未达 16px → `@media(max-width:640px){.dashboard{font-size:16px}}`；`--text-disabled` 白底仅 **2.54:1**(`glass.css:44`)→ 统一 `--text-muted`(#6b7280=4.83:1) |
| 指标 / 图表 / 状态卡 | emoji 密集(`ActivityTimeline.vue:30-67,89`、`Workbench.vue:78-83,208,272`、`QuickTools.vue:147,163,184,202`、`HealthStrip.vue:81`、`TrendSection.vue:102`) → 全换 SVG；`Sparkline.vue:66-68` 面积渐变、`TrendChart.vue:194` 磨砂 tooltip、`device-detail.css:107` hero 135° 渐变 → 实色 `--surface-1` + 纯色描边；图表/状态色裸 hex(`TrendSection.css:54-70`、`AutomationCard.vue:64-67`、`HealthStrip.vue:174-175,195,201,204,214,224,234,301,307`、`ActivityTimeline.vue:144-153`) → 新增 `--chart-1..4`、`--pt-*-fg` 语义 token | 图表仅 hover(`TrendChart.vue:95`、`Sparkline.vue:140-143`)、`RingChart.vue:53` SVG 无 role/aria → 加 `tabindex=0`、方向键切换 hoverIndex、`aria-live` 数值、`role="img"` + `<title>`；多数卡片加载中是纯文字 `dc-empty`(`AICard.vue:60-62`、`ActivityTimeline.vue:97`、`ResourceCard.vue:63-65`、`RealtimeCard.vue:68`)，仅 `Dashboard.vue:210-219` / `TrendSection.vue:54-58` 有骨架 → 统一 `el-skeleton`；`v-loading` 整块遮罩(`Workbench.vue:138`、`TaskCards.vue:154`、`Doctor.vue:81`) → 局部骨架 | **实测不达标**：`#d97706` 3.19:1、`#11998e` 3.52:1、`#2ecc71` 2.10:1、`#e74c3c` 3.82:1、`#94a3b8` 2.56:1(`RealtimeCard.vue:102,134,137`、`SyncCard.vue:149`、`HealthStrip.vue:204`) → 换 `--pt-warn-fg:#b45309` / `--pt-success-fg:#0f766e` / `--pt-danger-fg:#b91c1c`；数字缺 tabular-nums（仅 `ActivityTimeline.vue:180` 有）→ 给 `.metric-item strong`(`TrendSection.css:80`)、`.health-item b`、`.sync-percent`、`.s-num` 补；`.tl-item` 7px padding≈30px 行高 → `min-height:36px;padding:0 8px`；8px 网格被 3/6/7/10/14/18px 打散(`Dashboard.css:114`、`OpsDisk.css:35-39`) → 统一 `var(--sp-2/3/4)` |
| 工作台 / 工具 / 弹窗 | `.workbench.glass`(`Workbench.vue:138`)、`.quick-tools.glass`(`:142`)、`.widgets-empty.glass`(`Widgets.vue:116`)、`.sync-card.glass`(`SyncCard.vue:20`) → 统一 `.ops-surface`(`Dashboard.css:81`)/`solid-card`；`transition:all`(`DashboardWorkbench.css:70`、`OpsCenter.vue:124`、`OpsTerminal.css:66`) → 只列具体属性；`ExportDialog.vue:147` 的「glass」导出主题降为次要项 | TOTP/AES/网络输入仅 placeholder 无可见 label、非法 base32 静默返回 `'------'`(`QuickTools.vue:43,149,187`) → 补 `<label>`/`aria-label` + 就近行内错误；`<code @click>` 复制(`:153,172,175`)不可聚焦 → `<el-button text aria-label="复制">`；`genAes` 覆盖旧密钥无确认(`:93-98,169`) → 确认或保留历史；Widgets 仅 HTML5 DnD(`:126-129,164-166`) → 把已有 `moveWidget`(`:87`) 暴露为上下移按钮供键盘操作；remove 危险按钮(`:158`)配确认 | `.wb-score` 仅 `:title` 无 `aria-label`(`Workbench.vue:172-193`) → 补；节点单击筛选/双击跳转(`:205-206`)隐藏手势 → 单击筛选 + 显式「前往」链接；`.wb-event` 无 tabindex；`.nodata-chip` padding 2px 10px(`HealthStrip.vue:288-296`)≈20px → `min-height:24px` |
| EntertainmentOverview.vue | 6 类 emoji + 🎮 + chip(`:21-26,60,77,84,103`)、glass(`:64-89`)、hover translateY(`:126-127`)、裸 hex(`:135-138`) → SVG + `.solid-card` + 去位移 + token | `role="button"` 的 div 无 tabindex/键盘处理(`:76,90`) → `<button type="button">` 或补 `tabindex=0` + `@keydown.enter/space`；`.game-chip` padding 3px 10px(`:137`)≈22px → `min-height:24px`；`filterCat`(`:18,76,83`)不进 URL → `?cat=bet`；idleSites 仅标签无批量(`:110-115`) → 「批量去配置」`router.push` 带 `query.site=ids` | `.stat-card.dim{opacity:.45}`(`:127`)压低对比 → 改 `border-color` + `aria-pressed` 表达选中；`v-loading`(`:88`) → `el-skeleton`；空态调用的 `EmptyState.vue:46` 有 3s 无限浮动 → 去；12px 字(`:129,137`)移动端升 16px |
| OpsCenter.vue + Ops*.css | 渐变分隔线/容量条高光(`OpsDisk.css:116,221`)、绿点发光(`OpsMonitor.css:200-201`)、表头 blur(`OpsProcess.css:103`)、hover 上浮(`OpsTerminal.css:71`) → 全删，容量条用实色 `--pt-primary`，表头 `background:var(--surface-2)` | `activeTab` 纯 ref 无 URL(`:21,66-75`)，刷新回 monitor → `?tab=docker` + `router.replace`；移动端隐藏全部标签文字(`:156-159`)后图标按钮无 `aria-label` → 补 `aria-label`、`role="tablist"/"tab"/aria-selected` 与左右方向键；`.ops-tab` 6px 14px(`:116`)≈29px、移动 8px 10px(`:160-162`)≈36px<44px → `min-height:44px`；`.sort-btn/.quick-chip` 4px padding(`OpsProcess.css:72-80`、`OpsTerminal.css:58-68`)≈22px → `min-height:24px` | `--text-disabled` 2.54:1(`OpsDisk.css:31,79`、`DashboardWorkbench.css:102,156,208`)、`#e74c3c` 3.82:1(`OpsDisk.css:22`、`OpsTerminal.css:39`) → `--text-muted`/`--pt-danger-fg`；`OpsDisk.css:78,195` 11px → 12px；已有粘性表头+横向滚动(`OpsProcess.css:86-105`、`OpsSsh.css:103-126`，**正例**)，但 td 6px 10px(`OpsProcess.css:107`、`OpsSsh.css:122`)≈30px → `height:36px;padding:0 12px`；`.mono`(`OpsProcess.css:114`)加 tabular-nums；`var(--text-secondary,#888)` 双轨 fallback(`OpsDisk.css:18`) → 只留 token |
| NasServer / device-detail.css | glass-bar(`:203`)、glass 错误/空态(`:228,306`)、⚠/📦(`:229,307`)、color-mix 玻璃卡与渐变 hero(`device-detail.css:91-92,107`，白字对 `#11998e` 仅 **3.52:1**)、状态裸 hex(`:48-66`)、`#fff`(`NasServer.css:39`) → `.solid-card/--surface-1`、hero 改深阶实色 ≥4.5:1、emoji 换 `<el-icon>`、token 化 | 头部 7 个等权按钮(`:208-224`)无分组/溢出 → `el-dropdown` 收纳；`activeWorkspace/activeTab`(`:55-56`)无 URL、15s 轮询后丢失 → `?ws=deploy&tab=http`；批量执行(`:251-279` 已有 select-all，**正例**)推广到卡片列表(`:292-305`)多选批量编辑/删除；空态 CTA(`:312-317`)保留但去 emoji | `.tab-count` 背景 `--glass-bg`(`NasServer.css:55`) → `--surface-2`；`.empty-state` padding 48px(`:66`) → `var(--sp-6)`；`.info-row` 30px(`device-detail.css:325-330`) → 36px；`@media(max-width:760px)`(`:426-463`)补 `min-height:44px` |
| Doctor.vue | `.card.glass.finding`(`:82`)、`sevColor` 裸 hex(`:19,:84`)、staggered 6 段(`:158-178`)、hover translateX(`:182-184`) → 实色卡 + `--pt-success/warn/danger` token + 删装饰动画；状态 `.dot` 仅靠颜色(`:84,126-131`) → 加文字/图标 | 「一键修复」直接 `fixDoctor()` 无确认/dry-run/撤销(`:28-32,69`) → `ElMessageBox.confirm` 列出改动项 + 修复前快照/回滚；失败仅 `ElMessage.error`(`:34`) → 页内 `role="alert" tabindex="-1"` 可聚焦错误摘要并 focus；findings 为空整块空白(`:81-92`) → 补空状态；无筛选/批量处理 | `.f-rec{opacity:.7}`(`:146`)叠加 `--text-muted` 后对比不足 → 去掉 opacity；`.f-id` 12px(`:137`)移动端升 16px；`v-loading`(`:81`) → 骨架 |

**共性与优先级**
- **P0（阻断，必改）** ① **对比度实测 2.10~3.82:1**（`--text-disabled` 2.54、`#d97706` 3.19、`#11998e` 3.52、`#2ecc71` 2.10、`#e74c3c` 3.82、`#94a3b8` 2.56）→ 新增 `--pt-*-fg` 深阶 token 并全量替换；② 破坏性操作无确认/撤销（`Doctor.vue:69`、`ExportDialog.vue:239`、`QuickTools.vue:93-98`）→ 确认 + 快照/可撤销（`NasServer` 删除已有 confirm，见 `useDeviceCrud.ts:101`，勿重复）；③ 键盘可达（`EntertainmentOverview.vue:76,90` div role=button、`QuickTools.vue:153,172,175` `<code @click>`、`OpsCenter.vue:156-159` 隐藏文字图标 tab、`TrendChart.vue:95` / `Sparkline.vue:140-143` 仅 hover、`RingChart.vue:53` 无 aria）→ 语义 button/label/aria + tabindex + 方向键；④ 点击目标 22px 级 chip/周期/排序按钮、29~36px tab（移动需 44px）→ `min-height:24px/44px`。
- **P1** ⑤ emoji 全量换 SVG；glass/渐变/glow/staggered 入场统一实色（全局 reduced-motion 已兜底，应**直接删装饰动画**而非依赖兜底）；裸 hex → 语义 token；11px → 12px、移动端 → 16px；8px 网格、36px 行高、tabular-nums；⑥ 深链/URL 状态（Dashboard 周期/指标、OpsCenter tab、NasServer 工作区、EntertainmentOverview 分类）；加载反馈统一骨架；空状态/错误摘要补全；就近行内校验（QuickTools/ExportDialog）。
- **P2** ⑦ 每屏收敛到 1 个主 CTA、头部按钮分组溢出、批量操作推广、Tooltip 键盘化、glass 导出主题降级、`transition:all` 收敛。

**已有优点（勿重复整改）**：全局 `:focus-visible`(`glass.css:711`) 与 `prefers-reduced-motion`(`glass.css:659`)；Dashboard 骨架(`Dashboard.vue:210-219`)、TrendSection 骨架(`:54-58`)；粘性表头+横向滚动(`OpsProcess.css:86-105`、`OpsSsh.css:103-126`)；`?matrix=` 深链(`HealthStrip.vue:22-24`)；批量执行(`NasServer.vue:251-279`)；`OpsCenter.vue:73` 内联 SVG 单色图标。

> **四组审计覆盖完成**：A 仪表盘与总览 / B 列表表格内容 / C 配置表单流程 / D 全局外壳与跨端，共覆盖 47 个 Web view、`components/` 下全部相关子组件、Flutter 主题与代表页、扩展 popup/options。所有结论均为只读源码审计，带 `文件:行号` 证据。


#### B 组 · 列表 / 表格 / 内容（已完成）

> **最重要的正面结论**：`frontend/src/styles/glass.css:286-292` 的 `.solid-card` 注释原文就写着「信息密集页（插件/配置/列表）专用，替代 glass 的磨砂模糊——扫读时层次分明、文字不浮、**不像半成品**」。也就是说：**代码库里已经有正确答案，只是 20+ 个列表页没用**。同理 `.pt-skeleton`(L725)、`.pt-stagger`(L748)、`:focus-visible`(L711)、`prefers-reduced-motion`(L659) 已存在，B 组多数问题是「换成已有类/组件」而非新建。
> 路径约定：无前缀 = `frontend/src/views/`，`comp/` = `frontend/src/components/`。

**页面清单（17 组）**

| 文件 | 用途 | 视觉风格证据 |
|---|---|---|
| Sites.vue | 站点增删改 | head 5 CTA + SiteOperationsMatrix + dialog 表单 + 记录表；就近校验 `:rules` L205；删除无确认 L198-199 |
| SiteDetail.vue/.css | 站点详情评估 | 裸 hex 状态色 L63-67、✓ 字符 L281、css L131-155 stagger |
| Downloaders.vue/.css | 下载器管理 | GlassIcon 渐变 from/to hex L49-55/L269/L360、📥 L328、css L275-296 stagger |
| Transfer.vue/.css | 转移记录 | header 8 按钮 + 2 张 el-table；glass L148、📁 L255、`#f59e0b` css L111 |
| Reseed.vue/.css | 辅种任务 | toolbar.glass L80、3 个 GlassIcon 渐变 L93/97/104、裸 hex L93 |
| FileManager.vue/css | 文件浏览 | ✓/📁/🎬 L311/313、侧栏 135° 渐变 css L45、11px css L66/261 |
| MediaDetail.vue/.css | 媒体详情 | hero.glass + `blur(8px)` css L44、badge 135° 渐变 css L90、⭐🔍❤⚠ L124/133/134/92 |
| Search.vue | 种子/媒体搜索 | glass L234/266、⚠ L267 |
| Recommend.vue/.css | 推荐/开播日历 | 📅 L260/267/320/341/358、glass L234/298 |
| Subscriptions.vue/css | 订阅与批量编辑 | glass L256/260、🔖 L333、css L159-180 stagger；确认 L176（正面） |
| Rss.vue/.css | RSS 订阅 | glass L236/238、📡 L265、15 项表单无 `:rules` L278-336；确认 L147（正面） |
| Freeleech.vue/.css | 免费种聚合 | 🎉 L234 / 🔍 L240、状态色裸 hex L181-184 + 内联色 L218；tabular-nums css L51（正面） |
| Announcements.vue | 站点公告 | 📢 L91、site-group.glass L97 |
| Messages.vue | 站内信 | ✉️ L215、glass L171/184、`transition:all` L288 |
| Notices.vue/.css | 通知中心 | 🔔 L313、glass L230/253；单色 SVG L261/272（正面） |
| SiteRequests.vue | 求种 | card.glass L19、🙏 L55、`#f39c12` L140 |
| comp/search/* + comp/downloader/* | 种子卡/抽屉/表格 | 双层渐变 veil `torrentCard.css` L47/L54、促销 chip 裸 hex `torrentFindMore.css` L45-54、⬇ `TorrentPushDialog.vue` L150；10/11px `torrentTable.css` L35/95/105/153/160/193/213、th 背景 transparent L205-215 |

**逐页「现状 → 改法」**

| 页面 | 去 AI 感 | 交互便捷 | 一致性 / 可访问性 |
|---|---|---|---|
| Sites.vue | — | L161-182 头部 5 个同权重按钮 → 只留「添加站点」唯一 primary，其余收「更多」或并入矩阵批量条 | L198-199 全文件 0 处 confirm → 复用 `Rss.vue:147` + 撤销；L204-283 `width=480` 塞 11 个 form-item、L146 仅 `validate()` 无摘要 → 折叠「高级」+ `role=alert tabindex=-1` 摘要并聚焦首错 |
| SiteDetail.vue/.css | L63-67 裸 hex 状态色 → `var(--pt-success/warn)`；L281 ✓ → `SptIcon name=check` | L182/L214 返回固定 `push({name:'sites'})` 丢列表状态 → tab 与 `?site=` 写进 query 并带回 | css L131-155 手写 stagger → `.pt-stagger` |
| Downloaders.vue/.css | L49-55 from/to hex + L269/L360 GlassIcon 渐变方块（`GlassIcon.vue:9-17` 自述「iOS26 液态玻璃」）→ 单色 SptIcon + `--pt-primary-bg` 实色 | — | L328 📥；css L275-296 手写 8 段 animation-delay → `.pt-stagger --i` |
| Transfer.vue/.css | L148 `.glass` 承载数据表 → `.solid-card`（css L25 同名选择器同步改）；css L111 `#f59e0b` → `var(--pt-warn)` | L119-145 一行 8 个 small 按钮 → 留 L138「整理」唯一 primary，其余收「更多」，批量删除移入选中批量条；L120 viewMode / L124 statusFilter 纯内存 → `router.replace` 同步 `?view=&status=` | L141/L144 破坏性操作页内无确认 → 确认 + 撤销 |
| Reseed.vue/.css | L80 toolbar glass + L93/97/104 渐变 → `.solid-card` + 单色 SVG | L92-106 三按钮视觉接近、配置藏 popover → 留「开始辅种」唯一 primary，其余降 text | L208 `height=100%` 已固定表头（正面），css L100 th 无实色 → `--surface-1` |
| FileManager.vue/css | L311/313 ✓/📁/🎬 → SptIcon；css L45-49 选中 135° 渐变 → `--pt-primary-bg` 实色 | L269 删除无确认 → 确认 + 撤销（L221 selection 已有，扩批量条） | css L66/L261 11px、L71/L80 裸 `#f59e0b`、L67 `--text-muted,#909399` 灰字叠玻璃 → ≥12px + `--text-body` + `--pt-warn` + `.solid-card` |
| MediaDetail.vue/.css | L99 hero glass + css L44 `blur(8px)` + L54 overlay 渐变 + L90 badge 渐变 → 实色 `--surface-1` + 纯色低透遮罩 + `--pt-primary-bg`；L124/133/134/92 emoji → SptIcon star/search/heart/alert-triangle | — | L92 err-text 无 `role=alert`、L93 重试无 `aria-label` → 补 |
| Search.vue | L234/L266 glass → `.solid-card` | L277 仅 `:loading` 遮罩 → `.pt-skeleton` 按结果数占位防 CLS；keyword/mode/category → query 深链 | L267 ⚠ 纯文字错误、L262 空关键词可提交 → 就近 required 提示 + `role=alert tabindex=-1` + 重试聚焦 |
| Recommend.vue/.css | L260/267/274/320/341/358 emoji、L234/298 glass → 单色 SVG + `.solid-card` | source/category/calMode 内存态 → query | L236-275 原生 filter-chip 无 `min-height`/`aria-pressed` → 补 `aria-pressed`、32px（移动 44px）、显式 `:focus-visible` |
| Subscriptions.vue/css | L256/260 glass、L333 🔖、css L159-180 手写 stagger → `.solid-card` + SVG + `.pt-stagger` | L172/176 已有确认（正面）但无撤销 → 加「撤销」 | css L147 `.sub.selected` 仅 outline，勾选态与键盘焦点混淆 → 选中用实色边框/底，焦点交全局 `:focus-visible` |
| Rss.vue/.css | L236/238 glass、L265 📡 → `.solid-card` + SVG | L358 items 表无 height / 无 sticky / 无批量 → `max-height` + 实色表头 + 批量勾选 | L271-343 dialog 内 15 个 form-item（L278-336）无 `:rules/prop` → 逐项 rules（url/required）+ 错误摘要 + 聚焦首错 |
| Freeleech.vue/.css | L234 🎉 / L240 🔍、L208/248 glass → SVG + `.solid-card`；L181-184 裸 hex + L218 内联 `:style tile.color` → 语义 class | — | L209-220 tile 依赖 `v-a11y-button` 但无 `aria-pressed`/44px → 补齐；css L51 tabular-nums 推广到全部数字列 |
| Announcements.vue | L91 📢、L97 glass → SVG + `.solid-card` | L80 `v-loading="loading && !filtered.length"` 有数据时刷新零反馈 → 顶部进度条 / `aria-busy`；筛选同步 query | L111-119 展开项改 `button` + `aria-expanded` |
| Messages.vue | L215 ✉️、L171/184 glass → SVG + `.solid-card`；css L288/L331 `transition:all` → 收窄为 background/border-color | L172 tabs/筛选 → query | L220 drawer 打开未移入焦点、关闭未回原触发项 → focus trap + 返回焦点 |
| Notices.vue/.css | L313 🔔、L230/253 glass → SVG + `.solid-card` | L229-241 头部 5 操作 → 留「全部已读」为主，clearAll 收「更多」并确认（L182 已有 ElMessageBox，正面） | L231-236 filter-btn、L254-258 折叠头均为原生元素 + `v-a11y-button`，无 min-height / `aria-pressed` / `aria-expanded` → 补齐 32/44px 与 ARIA 状态 |
| SiteRequests.vue | L55 🙏、L19 glass、L140 `--pt-warn,#f39c12` → SVG + `.solid-card` + 纯 token | — | L27 el-table 无 height / sticky / 批量 → `max-height` + 实色表头 + 全部展开/折叠 |
| comp/search/* | `torrentCard.css` L47/L54 双层渐变 veil → 单层实色 + 1px 边框；`torrentFindMore.css` L45-54 四个裸 hex → 语义 tone class | `TorrentPushDialog.vue` L79 `v-loading` → `.pt-skeleton` | L150 ⬇ → SptIcon download；`TorrentCard.vue` L76/L84、`TorrentFindMoreDialog.vue` L42 原生 button 无 `aria-label`/32px → 补齐；`torrentCard.css` L127 tabular-nums 推广到 size/peers |
| comp/downloader/* | `StatsBar.vue` L62/64、`TorrentDrawer.css` L9、`torrentTable.css` L17/240 半透明 `--glass-border` → `--surface-border` 实色 | `TorrentTable.vue` L261 无 height 表头随滚动消失 → `max-height` / sticky thead；`AddTorrentPanel.vue` L130 URL、`TrackersDialog.vue` L56 tracker URL 无校验 → 就近 rules + 可聚焦错误摘要 | `torrentTable.css` L35/105/153/160/213 = 11px、L95/193 = 10px → 统一 12px + `--el-table-row-height:36px` + tabular-nums；L205-215 th 透明 → 实色 `--surface-1` + `position:sticky` |

**共性与优先级**
- **P0**（全组统一改，收益最大）① emoji 当图标 ≥14 处，几乎都经 `EmptyState.vue:18/24` 默认 📭 渲染，且 `:46 es-float 3s infinite` 浮动 → 把 `EmptyState` 的 `icon` prop 由 string 改成 SVG 组件（复用 SptIcon/el-icon），**一次替换覆盖全站**，图标静态化；② 装饰性毛玻璃/渐变 ≥20 处 `.glass`，而 `.solid-card` 注释明说列表页专用 → 列表/表格/内容页统一 `class="... glass"` → `class="... solid-card"`，去 GlassIcon 渐变方块与 135° 渐变，实色卡 + 8px 网格 + 36px 行高 + 12px 字 + tabular-nums；③ 破坏性操作缺确认/撤销（Sites.vue:198-199、Transfer.vue:141/144、FileManager.vue:269、SiteRequests.vue）→ 统一 `ElMessageBox.confirm`（范例 `Rss.vue:147`、`Subscriptions.vue:176`、`Downloaders.vue:155`）+ 撤销 toast。
- **P1** ④ 字号/对比度：`torrentTable.css` 10/11px、`filemanager.css` 11px 与 `--text-muted,#909399` 叠半透明玻璃底 → ≥12px + `--text-body` + 实色底；⑤ 每屏多主 CTA（Transfer.vue:136-144 8 按钮、Sites.vue:161-182、Notices.vue:229-241、Reseed.vue:92-106）→ 1 primary + 下拉收纳，32/44px 命中区，icon-only 补 `aria-label`；⑥ 表单校验（Rss.vue:278-336、AddTorrentPanel.vue:130、TrackersDialog.vue:56、Sites.vue:146）→ 就近 rules + `role=alert tabindex=-1` 摘要 + 聚焦首错；⑦ URL 状态（Transfer/Recommend/Notices/Messages/Rss/SiteRequests/Downloaders 全为内存 state）→ `router.replace` 同步 query；⑧ 加载反馈普遍仅 `v-loading`（Recommend.vue:293 甚至只有文字「加载中」）→ 铺 `.pt-skeleton` 并预留高度防 CLS；⑨ 表格（TorrentTable.vue:261、Rss.vue:358、SiteRequests.vue:27、torrentTable.css:205-215）→ `max-height` + 实色 sticky thead + `overflow-x:auto` + 批量选择条。
- **P2** ⑩ 手写 8 段 `animation-delay`（SiteDetail.css:131-155、Downloaders.css:275-296、subscriptions.css:159-180、Freeleech.css:125-146）→ 复用 `.pt-stagger`；移动断点命中区提到 44px 并补 `aria-pressed/aria-expanded`；⑪ 裸 hex 状态色（SiteDetail.vue:63-67、Downloaders.vue:49-55、Freeleech.vue:181-184、Transfer.css:111、filemanager.css:71/80、torrentFindMore.css:45-54）→ 语义 token。

**已达标、勿重复**：`glass.css`.solid-card(L286)/focus-visible(L711)/reduced-motion(L659)/tabular-nums(L211)/pt-skeleton(L725)/pt-stagger(L748)；SptIcon 单色 SVG（`Notices.vue:261,272`）；确认对话框范例（`Rss.vue:147`、`Subscriptions.vue:176`、`Downloaders.vue:155`）；`tabular-nums`（`Freeleech.css:51`、`torrentCard.css:127`）；`Reseed.vue:208` 固定表头；`BrushFlow.vue:270-282` 骨架。


#### C 组 · 配置 / 表单 / 流程（已完成）

**页面清单（22 组）**：Settings.vue（6 分区 × 嵌套 Tab，23 pane / 5 层嵌套）、GeneralPanel.vue/.css、Workflow.vue/.css、WorkflowEditor.vue/.css、NodeParamForm.vue、Plugins.vue/.css、PluginView.vue、PluginMarketSection.vue、4 个 Plugin*Dialog（另有 `components/plugins/PluginActionDialog.vue` 重复实现）、FilterRules.vue、Backup.vue、Cloud.vue/.css、Trakt.vue、Subtitle.vue、MediaSettings.vue、MediaQuality.vue、MediaServer.vue/.css、BrushFlow.vue/.css、Contract.vue、Assessment.vue、Harem.vue、MedalWall.vue/.css、Savept.vue/.css、dialog.css。

**视觉风格证据（精选）**：`Settings.vue:147,180` glass tabs 与 5 层嵌套 :148-252；`GeneralPanel.vue:51,166,185` glass ×3 + `styleGlass` 选项 :82 + 壁纸/遮罩 :86-139 + 22px accent-dot `GeneralPanel.css:27-28` + hover scale :38,42；`WorkflowEditor.vue:158-166` 📋⬆⬇▶💾、`animated:true` :203 无限边、`#aaa` :204；`Plugins.css:116,325` 头像渐变、:598-630 staggered、裸 hex :103,669-679；`PluginMarketSection.vue:110,117,124,137` ⭐🔥🥇🌱；`FilterRules.vue:91,137,181` 🧩✨🗂；`Subtitle.vue:123-126,144,176` 裸 hex + 渐变 + ⚠️；`MediaServer.vue:221,262` 渐变 hex + 🎬；`MedalWall.css:234-261` spring 入场；`Savept.vue:145` 站庆彩纸；`Savept.css:142,146,183` rgba(.4) 低对比；`dialog.css:90-125` 回弹「水润」动效 + 大量 `!important` :3-59。

**逐页「现状 → 改法」**

| 页面 | 去 AI 感 | 交互便捷 | 一致性 / 可访问性 |
|---|---|---|---|
| Settings.vue | :147 glass tabs → 实色 + 1px `var(--border)` | `applySettingsQuery` :86-116 只读 `?section/?tab`、切 Tab 不回写 → `onTabChange/onSectionChange` :119-140 内 `router.replace({query})` | 23 pane / 5 层 :148-252 → 二级侧栏 + `aria-current`；只读 URL :184-191 加复制按钮 |
| GeneralPanel | glass ×3 :51,166,185 + `styleGlass` :82 + 壁纸/遮罩 :86-139 → 删玻璃/壁纸项 | accent-dot 22px <24px 且仅 `:title` :71 → `min-width:32px` + `aria-label/aria-pressed` | help-row div + `v-a11y-button` :186,193,200 与 `›` :191 → 原生 `button` + Lucide ChevronRight |
| Workflow.vue/.css | 6 个 stat glass-hover :225-248 → 实色 | 每卡 2 个 primary :267-268 + 空态再 1 个 :285 → 每屏 1 主 CTA；:274 删除仅 `confirm` :103 → 加撤销；列表补批量与筛选深链 | `icon=🔀` :280 → SptIcon |
| WorkflowEditor.vue/.css | 📋⬆⬇▶💾 :158-166 → `el-button :icon` Lucide；`animated:true` :203 无限边 → `false`；`#aaa` :204 → `var(--border)` | `label.import-btn` + hidden input :160-163 键盘不可达 → `el-button` 触发 `fileRef.click()` | cat-item 仅 `draggable` + `:title` :184-191 → `tabindex`/`aria-label` 与键盘添加（skill `dragging-alternative`） |
| NodeParamForm.vue | ✕ :93 / ▼▶ :129 → Lucide X/ChevronDown | `window.prompt` :43-48 → 行内新增键输入框；`.catch(()=>{})` :31,37 静默失败 → `ElMessage.error` + 重试 | `f.help` 对 text/textarea 仅 placeholder :60,83,122 → `label` + `aria-describedby`；`required` :57 无 `:rules` → 就近校验 + 可聚焦错误摘要 |
| Plugins.vue/.css | 头像渐变 css:116、staggered :598-630 → 删；裸 hex `Plugins.vue:123` / css:103,669-679 → 语义 token | more-btn 纯图标 :220-222 无 `aria-label` → 补 | css:228 12px → `--fs-body`；表格 36px 行高 + tabular-nums；网格加批量启停 |
| PluginView.vue | `glass-border` :226 → `--border` | `min-height:560px` :223 → `60vh`；params 深链 :20-21 已有，补 `?tab=` 回写 | Refresh / No view / 错误文案硬编码 :176,189,143,153 → i18n；`background:#fff` :228,236 破坏暗色 → `var(--surface)` |
| 4 个 Plugin*Dialog | — | 表格 `size=small` 保留 `max-height`（PluginAuditDialog.vue:196,207 / PluginLogsDialog.vue:80）+ 粘性表头 + `overflow-x` | **双实现**：`views/PluginActionDialog.vue:34-68` 与 `components/plugins/PluginActionDialog.vue:33-59` → 删其一，统一 `defineModel` 版；PluginConfigDialog.vue:73 图标关闭补 `aria-label` |
| PluginMarketSection.vue | ⭐🔥🥇🌱 :110,117,124,137 + 万灵阁/百宝袋/百鸟朝凤 :100,121 → 全部/收藏/榜单 + Lucide Star/Flame/Trophy | `marketView` 无 URL 状态 :102-117 → `?view=` 同步 | `chart-rank` 22px `Plugins.css:657-658` → 28px |
| FilterRules.vue | 🧩✨🗂 :91,137,181 → Lucide；glass :66,187,223 → 实色 | 试算输入仅 placeholder :189-195 → `label`；表格 :69,104,150 无 `max-height` → 480 + 粘性表头 + `overflow-x:auto` | link 按钮 :128-129 <24px → `min-height:24px` + 批量删除 |
| Backup.vue | glass card ×4 :240-276 → 实色 | 三个「选择文件」同文案 :259,271,283 且 `:show-file-list=false` :258,270,282 → 显示文件名/大小/进度 + 拖拽区；**导入覆盖不可撤销**（:114-124 注释自认）→ 导入前自动快照 + 撤销 | 二次确认 :33,119 已达标 |
| Cloud.vue/.css | `GlassIcon` :22 渐变 → 单色 | `el-tabs` :350 无 URL → `?tab=`；mkdir/offline/share/pwd 连续 4 次 `ElMessageBox.prompt` :59-88 → 分步表单 | `Cloud.css:12 --fs-xs` → `--fs-body`；任务表加骨架 |
| MediaSettings.vue | card glass :85 → 实色 | 6 组字段仅底部 1 个保存 :194-198 → 每组就地保存 + 「未保存」脏标记；`el-input` 无 `:rules` → 域名/Key 就近校验 | `pt-chip` 状态 :105-108 已达标 |
| MediaQuality.vue | glass :93,121 / GlassIcon :148 → 实色 + Lucide | 页头保存 :88 与试算查询 :143 双 primary → 仅 1 个；`ElMessage.warning` :66 顶部校验 → `el-form-item :rules` 行内 | `label-width=140` :95 已达标 |
| MediaServer.vue/.css | 渐变 hex :221、🎬 :262、glass glass-hover :219 → 单色 Lucide + 实色 | `nameTypeUrlRequired` 仅 `ElMessage.warning` :109,118 → `el-form rules` | 删除确认 :133 已达标 → 补撤销 |
| Trakt.vue | 标题内 GlassIcon :164 / glass status-card :178 → 移除 | 授权轮询仅 `ElMessage` :117,126,131 → 状态卡内联进度 + 可聚焦错误摘要 | `label-width=150` :210 已达标，补 required 与错误定位 |
| Subtitle.vue | 4 源裸 hex :123-126、渐变 :144、⚠️ :176 → `--pt-*` + Lucide | 仅 `v-loading` :154 无骨架、空态无「去配置源」CTA | `@keyup.enter` :145 已达标；结果列表补 tabular-nums |
| BrushFlow.vue/.css | 🚀 :329、渐变 :290、staggered css:89-108 → 删；card:hover `translateY` css:115-117 → 仅阴影 | `nameSiteDlRequired` :132 顶部校验 → 行内；`?site_id=` 深链 :22-24 已达标，补筛选回写 | 骨架 :270-282、`max-height=420` :358 已达标 |
| Contract / Assessment / Harem | 🔖⚠📋🔎💗（Contract:228,213 / Assessment:209,215 / Harem:178）、glass Contract:177 / Assessment:166 / Harem:100,131 → 实色数据卡 | 表格无粘性表头 `Harem:132` → `max-height` | 裸 hex `Harem:146,152,211` / `Assessment:95-99,199,269` → `--pt-success/danger/warn`；✓/✗ `Harem:152` → Lucide Check/X；`Assessment:343` tabular-nums 已达标 |
| MedalWall.vue/.css | 🎖️ :247、spring 入场 css:234-261、hex :90-97,264 → 实色 + 语义 token + Lucide | 购买确认 :117 已达标 → 加撤销/退款入口 | `el-drawer` :252 补 `aria-label` 与焦点归还 |
| Savept.vue/.css | 站庆彩纸 :145（`useSaveptCelebration.ts:151` 已判 reduced-motion）→ 静态徽标 | 自建 `--spt-*` css:1-19 双 token 体系 → 并入 `--pt-*` | `rgba(...,0.4)` 文本 css:142,146,163,183 对比 <4.5:1 → `--text-muted`；`0.75rem` css:145 → ≥12px；半透明焦点环 css:123 → `2px solid var(--pt-primary)`；`useSaveptCelebration.ts:151` 与 css:121-124 焦点环已达标 |
| dialog.css | 「水润/回弹」装饰动效 :90-125 → opacity 120ms 线性 | :3-59 大量 `!important` → 收敛为 `--el-*` 变量 | `.dialog-fade-*` 的 `!important` :93-124 优先级**高于**全局 `prefers-reduced-motion`（glass.css:659-665）→ 在 dialog.css 内补 `@media (prefers-reduced-motion:reduce)` |

**共性与优先级**
- **P0** ① 去 AI 感：emoji 作图标（`EmptyState.vue:24` 直接渲染 `{{ icon }}`、默认 📭 :18）+ 毛玻璃/渐变/彩纸/staggered/spring 入场遍布全组（`Plugins.css:116,598-630`、`MedalWall.css:234-261`、`Savept.vue:145`、`dialog.css:90-125`）→ 统一实色卡 + Lucide + 8px 网格；② **表单无就近校验/错误摘要**：全组零 `:rules/validate`，仅 `ElMessage.warning`（`MediaServer.vue:109`、`BrushFlow.vue:132`、`Workflow.vue:197`、`MediaQuality.vue:66`）→ `el-form :rules` + `aria-invalid` + 可聚焦错误摘要；③ `dialog.css` 动效绕过全局 reduced-motion。
- **P1** 语义色裸 hex（`Plugins.vue:123`、`Assessment.vue:95-99`、`MedalWall.vue:90-97`、`Subtitle.vue:123-126`、`Plugins.css:669-679`）；URL 状态不回写（`Settings.vue:87` 只读、`Plugins.vue:140-147`、`Cloud.vue:350`、`PluginMarketSection.vue:102-117`）；表格缺粘性表头（`FilterRules.vue:69,104,150`、`Harem.vue:132`）；点击目标 22px（`GeneralPanel.css:27-28`、`Plugins.css:657-658`）；多主 CTA（`MediaQuality.vue:88,143`、`Workflow.vue:267-268`）；破坏性操作缺撤销（`Workflow.vue:274`、`Backup.vue:114`）与批量操作；多数页仅 `v-loading` 无骨架。
- **P2** 双实现重复（两个 `PluginActionDialog.vue`）；`PluginView.vue` 硬编码英文；`--fs-xs/--fs-sm:12px` 与 12px 小字（`Plugins.css:228,663`、`Savept.css:145`）→ 正文 ≥14px / 移动 16px；`v-a11y-button` div 代原生 button（`GeneralPanel.vue:186`、`NodeParamForm.vue:128`）；hover transform/scale（`BrushFlow.css:115-117`、`GeneralPanel.css:38`）；`Savept.css:1-19` 自建 token 体系。

**已达标（保留）**：全局焦点环 `glass.css:711`；全局 reduced-motion `glass.css:659`；tabular-nums `glass.css:211` / `Assessment.vue:343`；骨架 `BrushFlow.vue:270-282`；二次确认 `Backup.vue:33,119` / `Cloud.vue:46,207` / `MedalWall.vue:117`；深链 `BrushFlow.vue:22-24` / `Savept.vue:151-161` / `PluginView.vue:20-21`；粘性表头 `BrushFlow.vue:358` / `PluginAuditDialog.vue:196`；`Savept.css:121-124` 焦点环 + `useSaveptCelebration.ts:151`。




---

## 7. 组件与交互改造的代码级约定（示例）

~~~ts
// useFeedback.ts —— 统一 464 处 ElMessage/ElMessageBox
export function useFeedback() {
  const toast = (msg: string, type: 'success'|'warning'|'error'|'info' = 'success') =>
    ElMessage({ message: msg, type, duration: 4000, showClose: true }) // aria-live 由 EP 承担
  const confirm = (opts: { title: string; detail: string; danger?: boolean }) =>
    ElMessageBox.confirm(opts.detail, opts.title, {
      type: opts.danger ? 'warning' : 'info',
      confirmButtonClass: opts.danger ? 'el-button--danger' : '',
      distinguishCancelAndClose: true,
    })
  const undo = (msg: string, action: () => void) =>
    ElMessage({ message: msg, type: 'success', duration: 8000,
      showClose: true, action: () => action() })  // 破坏性操作后 8s 可撤销
  return { toast, confirm, undo }
}
~~~

~~~css
/* PtTable 基线（data-dense-dashboard 验证值） */
.pt-table { --row-h: var(--table-row-height); font-size: var(--fs-sm); }
.pt-table thead th { position: sticky; top: 0; z-index: 2;
  height: var(--row-h); background: var(--color-surface-2);
  border-bottom: 1px solid var(--color-border); font-weight: 600; }
.pt-table tbody tr { height: var(--row-h); }
.pt-table tbody tr:hover { background: var(--color-surface-hover); } /* 只变背景 */
.pt-table .num { font-variant-numeric: tabular-nums; font-family: var(--font-mono); }
.pt-skeleton { min-height: var(--row-h); } /* 固定占位，CLS < 0.1 */
~~~

~~~css
/* 表单：就近错误 + 可聚焦错误摘要（skill: error-placement / error-summary / focus-management） */
.pt-field__error { color: var(--color-danger); font-size: var(--fs-xs);
  margin-top: var(--space-1); }
.pt-field__input[aria-invalid='true'] { border-color: var(--color-danger); }
.pt-error-summary { border-left: 3px solid var(--color-danger);
  background: var(--color-danger-soft); padding: var(--space-3); }
/* 提交失败后：focus 到 .pt-error-summary（tabindex=-1），每条链接到对应字段 id */
~~~

---

## 8. 交互逻辑专项（「方便」的 10 条硬规则）

1. **加载反馈**（`loading-indicators` / `content-jumping`）：所有异步 >300ms 显示骨架或进度；<300ms 不闪 spinner；骨架固定高度；`aria-busy`；长任务显示「步骤 2/4」；`TaskIndicator` 可展开任务详情。
2. **表单**（`inline-validation` / `error-summary` / `submit-feedback`）：blur 校验（非逐字）、错误在字段下方并 `aria-describedby` 关联、提交后 loading→成功/失败、多错误时顶部可聚焦摘要并链接到字段、失败保留已填内容、复杂字段常驻 helper 文本、必填标记、长表单草稿自动保存。
3. **表格与批量**（`bulk-actions` / `table-handling`）：多选 + 批量操作栏（目前 250+ 表格几乎都是逐行操作）；列显示/顺序可配置并持久化；排序/筛选/页码/关键词写进 URL（可分享、后退可恢复）；窄屏横向滚动容器 `overflow-x:auto`，不撑破布局；>50 行虚拟滚动。
4. **破坏性操作**（`confirmation-dialogs` / `undo-support`）：删除/清空/重置必须确认并写明对象名称与后果；可撤销的给 8s `撤销` toast；危险按钮与主要按钮分开且视觉弱化。
5. **导航可预测**（`back-behavior` / `deep-linking` / `breadcrumbs`）：站点详情 `/sites/:id`、插件视图、工作流编辑器加面包屑；Tab/筛选/展开态写 query；保留滚动位置；顶栏固定时 `scroll-padding-top`（`sticky-navigation`）。
6. **键盘与焦点**：⌘K/CTRL+K 已有（`CommandPalette.vue:165`）→ 补 `/` 聚焦搜索、`Esc` 逐层关闭、`g+d` 等跳转；所有图标按钮 `aria-label`；弹窗焦点陷阱；焦点不被吸顶栏遮挡（`focus-not-obscured`）。
7. **通知与状态**（`toast-accessibility` / `active-state`）：toast 3–5s 自动消失、不抢焦点、`aria-live="polite"`；统一「处理中/成功/失败/部分失败」四态并配图标+文字，不靠颜色（`color-not-only`）。
8. **空态**（`empty-states`）：`PtEmptyState` = 线性图标 + 「为什么空」+「下一步做什么」按钮；移除 `📭` 与浮动动画；首次使用的引导只出现一次。
9. **图表**（`--domain chart`）：折线用实线/虚线/点线 + 直接系列标签，不只靠颜色；提供可见数据表兜底；键盘可聚焦读数；\<4 个数据点改用数值卡；`Sparkline` 组件补 `aria-label` 与数值表。
10. **响应式**：统一断点 375 / 768 / 1024 / 1440（现在 25 处 `@media` 各写各的）；移动端正文 ≥16px、输入 ≥16px 防 iOS 缩放、点击目标 ≥44px、无横向滚动；`prefers-reduced-motion` 全量验证。

---

## 9. 跨端一致性

### 9.1 Flutter（`mobile/lib/theme/glass.dart`）
- `PTColors.primary: #6C5CE7 → #2563EB`（暗 `#4C8DFF`），新增语义色与 `--color-*` 同名常量；`PTFs.body: 13 → 14`（输入 16）；`PTRadius.lg: 20 → 8/10`。
- 删除 `GlassBackground` 渐变（`:104–117`），`scaffoldBackgroundColor` 改实色 `Color(0xFFF8FAFC)` / 暗 `#0F1115`。
- `GlassCard`（`:180+`）：`BackdropFilter` 仅保留给 modal scrim；`blurSigma` 默认 0；`padding: 20 → 12/16`；`radius: 20 → 8`；移除 `_GlassTapFeedback` 的「按进液态玻璃」下沉缩放，改标准 `InkWell` 高亮。
- 页面转场 `_PTLiquidPageTransitionsBuilder`（`:60–75`）→ 平台标准（Material/Cupertino）；`toolbarHeight: 44 → 48/56`（skill 触摸目标 44pt/48dp 下限）。
- `glassReduced` 逻辑保留（性能降级仍有用），但默认 `true`。

### 9.2 浏览器扩展
- `extension/theme.css` 升格为**共享 token 命名源**：把 `--ink/--bg-raised/--r-*` 别名到 `--color-text/--color-surface/--radius-*`，Web 端反向对齐；避免两套命名长期分叉。
- `popup.html`：10.5/11/11.5px → ≥12px；`.icon-btn 26×26` → 32×32（web 24px 下限，触摸 44px 建议）；`.dlitem .ops button padding:1px 8px` → ≥6px 高命中区；`.avatar 22px` 保持但加 `aria-label`。

---

## 10. 验收标准与自动化门禁

**A. skill Pre-Delivery Checklist（逐项打勾）**：无 emoji 图标 ✓ 单一图标族 ✓ 按下反馈不改变布局 ✓ 语义 token 无裸 hex ✓ 无装饰模糊/渐变 ✓ 100% 图标按钮有 accessible name ✓ 点击目标 ≥24px（移动 ≥44px）✓ 焦点可见且不被遮挡 ✓ 表单 label/helper/就近错误 + 可聚焦错误摘要 ✓ 不靠颜色传意 ✓ 暗色独立复核 4.5:1 ✓ reduced-motion ✓ 375/768/1024/1440 无横向滚动 ✓ 弹窗/抽屉不遮挡焦点 ✓ 拖拽有按钮/键盘替代 ✓ 登录支持密码管理器与粘贴。

**B. 可量化门禁（CI）**

| 检查 | 阈值 | 实现 |
|---|---|---|
| 组件裸 hex | 0 | stylelint `color-no-hex`（仅 token 文件豁免） |
| `backdrop-filter` | 仅 modal scrim | stylelint `declaration-property-value-disallowed-list` |
| 装饰渐变 | ≤3 处（进度/图表） | grep 门禁 |
| UI emoji / confetti | 0（娱乐页白名单需登记） | CI grep + i18n 扫描 |
| 无限装饰动画 | 0 | grep `infinite` |
| `font-size: 1[01]px` | 0 | stylelint |
| 图标按钮无 `aria-label` | 0 | eslint `vue/require-aria-label` 自定义规则 |
| 新增 `el-*` 静态引入 | 0 | 扩展 `scripts/check-no-new-ep.mjs` 棘轮 |
| 对比度 | 全部 token 对 ≥4.5:1 | 新增 `scripts/check-contrast.mjs` |
| 视觉回归 | 375/768/1024/1440 各主题 | Playwright（已有 `frontend/e2e/core-flow.spec.ts`、`extended-flow.spec.ts`）+ axe-core 扫描 |
| CLS | <0.1 | 骨架固定高度 + 图片宽高 |

---

## 11. 落地路线图与风险控制

| 阶段 | 工期 | 内容 | 风险与对策 |
|---|---|---|---|
| 0 · 换肤 | 2–3 天 | P0-1~P0-3 + 兼容层 + `classic` 开关 | **最大收益最低风险**：只改 token 与全局 CSS，页面 template 不动；灰度用 `data-ui-style` 开关，老用户可一键回退 |
| 1 · 全局外壳 | 1 周 | P0-4~P0-8 + §4.4 组件层 + 图标/emoji/i18n 清理 | 导航 `from/to` 字段删除需同步 `useMainLayout.ts` 的 `PaletteItem` 与 `CommandPalette.vue`；i18n 中英双语同步 |
| 2 · 逐页 | 1–2 周 | §6 四组页面按 `PtTable/PtFormField/PtEmptyState/PtDialog` 收敛，ratchet 只减不增 | 按「新页面/改动页必须过 lint」推进，存量分批；不追求一次清零 |
| 3 · 交互 | 1 周 | §8 十条 + 暗色复核 + 响应式 | URL 状态改动需 E2E 覆盖后退/刷新 |
| 4 · 跨端与门禁 | 1 周 | §9 + §10 CI | Flutter 需双端截图回归；扩展 popup 尺寸受 378px 限制，放大字号要同步调整布局 |

**明确不做**：不改后端 API、不改导航信息架构（`design/menu-redesign.md` v2 已定稿）、不删除娱乐/游戏功能（只隔离与降噪）、不强推新字体包（中文优先系统字体，避免 FOIT/CLS）、不引入新 UI 框架（Element Plus 存量按需保留，新页面走 `ui/` 组件层）。

---

## 12. 附录 · 证据索引

| 主题 | 证据 |
|---|---|
| 液态玻璃定位 | `frontend/src/styles/glass.css:1–9` |
| 紫蓝主色/渐变 | `glass.css:17–24, 400–430, 210–217` |
| 磨砂实现 | `glass.css:28–32, 236–262` |
| 圆角/字号阶梯 | `glass.css:38–58` |
| 装饰动效 | `glass.css:560–780`；`dialog.css:100–125`；`EmptyState.vue:40–50` |
| 主题/壁纸个性化 | `useUiPrefs.ts:22–40, 88–105`；`GeneralPanel.vue:66,78–149`；`glass.css:~790–860` |
| 彩虹导航 | `navConfig.ts:23–100`；`AppSidebar.vue:~80` |
| AI 徽标 | `MainLayout.vue:107–116, 285`；`Assistant.vue:171,203` |
| Emoji | `BigSmallSidebar.vue:44,72`；`DashboardActivityTimeline.vue:33,47`；`SearchFilterBar.vue:135`；`SpinnerGame.vue:32`；`zh-CN.ts:268,853,2750,2949,4166,4279` |
| 小字号 | `torrentTable.css:95,193`；`filemanager.css:66,261`；`OpsDisk.css:78,195`；`extension/popup.html` |
| 正面先例 | `extension/theme.css:1–45`；`glass.css:~660`（focus/reduced-motion）；`dialog.css:1–60`；`main.ts`；`router/index.ts`；`scripts/check-no-new-ep.mjs` |
| 移动端 | `mobile/lib/theme/glass.dart:10–14, 60–75, 104–117, 180+` |
| 密度基准来源 | skill `--domain style` → `data-dense-dashboard`；`--design-system` 第 2 次；`references/quick-reference.md` §5–§9 |
---

## 13. 实施记录（本轮已落地代码）

> 本节记录「全部解决」这轮真实代码改动。方案 §1–§12 是审计与设计，§13 是落地结果。
> 所有改动保持向后兼容：功能、路由、API、业务数据、i18n key 均不变。

### 13.1 实施策略：换肤而非重写

没有逐页重写 47 个 view，而是分三层推进，风险与改动量都可控：

1. **语义层**：新增 `frontend/src/styles/tokens.css`（三端唯一真源）。
2. **兼容层**：新增 `frontend/src/styles/modern.css`，把存量 `--pt-* / --glass-* / --grad-* / --radius-* / --fs-*` 别名映射到新 token，并把 `.glass / .glass-bar` 覆盖为实色卡。47 个页面 template 无需改动即整体换肤；`main.ts` 加载顺序为 `tokens.css → glass.css → dialog.css → modern.css`。
3. **组件层**：新增 `useFeedback` / `useQueryState` / `useUrlState` / `PtSkeleton`，扩展 `SptIcon` 到 136 个单色 Lucide 字形，各页面按需接入。

`glass.css` 已标注为 LEGACY（保留类名兼容），`classic-ui` 作为过渡期逃生开关保留 1–2 个版本。

### 13.2 已落地清单

| 模块 | 落地内容 |
|---|---|
| **全局外观** | 中性实色表面、1px 边框、单一信任蓝、无全屏渐变/壁纸/磨砂/rim 高光；spring 回弹、水波纹、stagger 入场、数字脉动、链接下划线展开全部停用；`--color-on-accent` 解决暗色亮蓝底白字 3.20:1 → 5.92:1；Element Plus 内置组件变量（`--el-color-primary` 等）统一跟随语义 token；石墨灰主题色预设补齐实际样式 |
| **对比度** | 实测不达标的 10+ 组颜色组合（最低 2.10:1）改为 `--pt-*-fg` / `--color-text-muted` 等深阶 token；浅色/深色各 12 组 token 组合全部 ≥4.5:1 |
| **导航与外壳** | 删除 `NavItem.from/to` 彩虹渐变（34 项）、`GlassIcon` 改单色线性图标、侧栏 240px 与 2px 主色激活条、顶栏「AI」渐变胶囊改单色 bot 图标、⌘K/Ctrl K 按平台显示、用户菜单改原生 button、登出确认 |
| **偏好设置** | `uiStyle` 默认 `modern`；壁纸 / 遮罩 / 卡片风格 / Bing 每日壁纸全下线并清理历史 DOM 残留；5 套主题色收敛为「默认蓝 / 石墨灰」；历史值自动迁移；主题明暗纳入 `useUserPrefs` 服务端同步 |
| **全局组件** | 命令面板去 `outline:none` / 误挂 `v-a11y-button`、补 `aria-activedescendant` 与焦点陷阱；通知中心去无限摇铃、单条删除/全部已读补确认+撤销；告警条改浅底深字；状态点静态化；空态去 📭 与 3s 浮动；错误浮层去 emoji 并 token 化（保留字面回退以自给自足） |
| **交互基建** | `useFeedback`（确认 / 8s 撤销 / 带恢复路径的错误）、`useQueryState(s)`（筛选/Tab/视图/周期写回 URL，无 router 时降级为本地 ref）、`PtSkeleton`（固定高度防 CLS）；统一 90+ 单色 SVG 图标集 |
| **逐页改造** | 仪表盘/运维/NAS/诊断、登录/助手/娱乐、列表/表格/内容、配置/表单/流程、Flutter 移动端、浏览器扩展分组推进：emoji→SptIcon、裸 hex→语义 token、渐变/发光→实色、`glass`→`solid-card`、小字号→≥12px（移动 16px）、多主 CTA 收敛为 1、表格粘性表头/36px 行高/批量条、表单 rules + 就近错误 + 可聚焦错误摘要、破坏性操作确认+撤销、URL 状态、骨架加载、键盘可达与 ARIA 状态 |
| **Flutter 端** | `glass.dart` 实色主题、去 BackdropFilter 与自写液态转场、正文 13→16、工具栏 44→48、`glass_icon.dart` 单色线性、`pt_motion.dart` 去 spring/无限呼吸/stagger（默认关闭并尊重 `MediaQuery.disableAnimationsOf`）；20+ 页面 emoji 与小字号清理 |
| **扩展端** | `theme.css` 补 `--color-*` 别名与三端统一命名；`--ink-3` 提亮到 5.8:1；tabbar emoji → 内联 SVG、5 格改「4 导航 + 1 独立 CTA」；10–11.5px → ≥12px；按钮 ≥32px；`#status` 补 `role="status" aria-live="polite"` |
| **数据层** | `dockerTemplates.ts` / `composeTemplates.ts` 的 95 个 emoji 图标改为 SptIcon name，消费处改 `<SptIcon>` 渲染；工作流节点分类色 / 仪表盘导出图色改语义 token |

### 13.3 工程门禁（CI 已接入）

在 `.github/workflows/ci.yml` 的 frontend job 增加 `npm run lint:ui`、`npm run check:contrast`、`npm run lint:css:ci`、`npm run lint:ep`：

- `scripts/check-ui-a11y.mjs`：棘轮门禁，统计 emoji / `outline:none` / 10–11px 小字 / 渐变 / 装饰模糊 / 无限动画 / `transition:all` / 裸 hex / 品牌色 / views 静态引入 element-plus 共 10 项，只许减不许增，指标下降时自动收紧基线 `scripts/ui-a11y-baseline.json`。
- `scripts/check-contrast.mjs`：浅/深色 token 前景-背景组合全部 ≥4.5:1（含按钮文字与四种语义色）。
- `frontend/stylelint.config.mjs`：新增渐变/小字号/transition:all/无限动画/装饰模糊的 advisory 规则；原有品牌色防回流规则保留。

**门禁指标变化**（基线随改造自动收紧，最终值）：emoji 636→70（剩余为游戏内容符号与 LEGACY 兼容层）、`outline:none` 5→0、10/11px 小字 45→0、渐变 102→18（剩余为转盘功能性配色与 LEGACY）、装饰模糊 14→6（仅 LEGACY/classic 逃生开关）、无限动画 22→10（仅加载指示器与 LEGACY）、`transition:all` 40→0、裸 hex 1514→798、品牌色 15→6（仅 LEGACY）。

### 13.4 验证结果

| 命令 | 结果 |
|---|---|
| `npm run typecheck`（vue-tsc） | **exit 0**，0 errors |
| `npm test`（vitest） | **111 files / 520 tests 全通过，0 unhandled errors** |
| `npm run build`（vite） | **exit 0**，2580 modules，35.8s |
| `npm run lint`（eslint） | **exit 0**，0 errors（374 条存量 warning） |
| `npm run lint:css:ci`（stylelint） | **exit 0** |
| `npm run lint:ui` | **PASS**，619 个 UI 文件全部指标未回潮 |
| `npm run check:contrast` | **PASS**，浅深各 12 组 ≥4.5:1 |
| `npm run lint:ep` | 存量棘轮无新增（随 CI 执行） |
| `dart analyze lib`（Flutter） | **No issues found!** |
| `flutter test test/glass_test.dart` | **8/8 通过** |
| `node --test extension/test/*.test.mjs` | **100/100 通过**（含 emoji 清理后同步的断言） |
| `npx playwright test`（E2E，真实 Go 后端 + mock qB） | **8/8 通过**：登录、添加站点、跨站搜索 SSE、创建下载器/测连/推送/任务、i18n 切换持久、订阅增删（含确认）、登录失败可见错误、登出确认 |

**过程中修复的问题**：`useUrlState` 的 `ComputedRef` 只读签名导致 5 处赋值类型错误 → 改 `WritableComputedRef`；无 router / 部分 mock router 的单测环境访问 `route.query` 或 `router.replace` 抛错 → `safeRouter()` 校验后退化为本地 ref；`modern.css` 自定义属性自引用循环 → 改字面值；暗色按钮白字 3.20:1 → 引入 `--color-on-accent`；站点表单误把「定义」设为必填导致核心添加站点流程被静默拦截 → 恢复可选并在校验失败时聚焦首错；登出新增确认框后同步 E2E 断言；`Set-Content` 破坏 UTF-8 文件 → git 恢复并用 edit 重做。

### 13.5 保留项与已知限制

- `glass.css` 保留为 LEGACY 兼容层，其中遗留渐变/磨砂规则在默认外观下被 `modern.css` 覆盖、不生效；`classic-ui` 逃生开关可临时恢复旧观感。
- 娱乐游戏组件的**内容符号**（转盘扇区、奖品、农作物/动物、骰面、爻符号等）保留并加 `aria-hidden="true"`；结构性 UI 图标（标题/按钮/徽标/状态/空态）一律换单色 SVG。转盘功能性 conic-gradient 配色保留。移动端娱乐游戏视图的装饰符号按「只隔离降噪」保留。
- 浏览器扩展的结构性 emoji 已清理为纯文字/内联 SVG，相关静态测试断言（push_history_note / push_batch_reason / favorite_batch / inject_all_tabs）已同步更新并通过；仅保留速率方向 ↑/↓ 与说明文字中的箭头等非 emoji 符号。
- 备份导入前会自动导出一份当前配置快照（`ptpatronus-pre-import-snapshot.json`）供回滚；后端无「导入撤销」API，故不做内存级 undo。
- 通知/收藏等「撤销」为本地状态回填（后端无恢复单条/恢复未读接口），刷新后以服务端为准；如需服务端真撤销需新增 API。
- 站庆彩带由无限循环改为一次性爆发，并尊重 `prefers-reduced-motion`；如需完全静态可进一步移除 canvas-confetti。
- 两个 `PluginActionDialog.vue` 重复实现保留（避免破坏引用），建议后续统一为 `defineModel` 版。
- 门禁为**相对棘轮**（只许减不许增），不要求一次性清零；存量裸 hex / 渐变按页面迁移逐步消化。

### 13.6 文档与说明同步

- `README.md`：仓库结构说明、UI 设计系统入口与门禁命令。
- `CHANGELOG.md`：新增 Unreleased 全站 UI/交互改版条目（含升级注意与迁移说明）。
- `design/ui-ux-redesign-plan.md` 本文件：§1–§12 审计与方案，§13 实施记录。



