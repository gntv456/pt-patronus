// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 导航配置数据：侧栏分组、palette-only 项、描述、别名

export interface NavItem {
  name: string
  label: string
  icon: string
}
export interface NavGroup {
  /** locale 查找键（nav.groups.<key>）；title 仍是折叠状态的持久化键（稳定不随 locale 变） */
  key?: string
  title: string
  items: NavItem[]
}

// P2: 侧栏收敛到定稿主菜单；移出项保留在 paletteOnly（⌘K/收藏/深链）。
// 图标统一由 SptIcon / GlassIcon 按单色描边渲染，不再携带每项独立的渐变配色。
export const navGroups: NavGroup[] = [
  {
    key: 'overview',
    title: '概览',
    items: [
      { name: 'dashboard', label: '仪表盘', icon: 'dashboard' },
      { name: 'search', label: '搜索', icon: 'search' },
      { name: 'recommend', label: '推荐', icon: 'recommend' },
      { name: 'subscriptions', label: '订阅', icon: 'subscribe' },
      { name: 'sites', label: '站点', icon: 'sites' },
    ],
  },
  {
    key: 'activity',
    title: '动态',
    items: [
      { name: 'notices', label: '通知中心', icon: 'bell' },
      { name: 'messages', label: '消息', icon: 'message' },
      { name: 'announcements', label: '公告', icon: 'announcement' },
      { name: 'site-requests', label: '求种', icon: 'seed' },
    ],
  },
  {
    key: 'site',
    title: '站点活动',
    items: [
      { name: 'entertainment', label: '娱乐', icon: 'entertainment' },
      { name: 'medal-wall', label: '勋章墙', icon: 'medalwall' },
      { name: 'harem', label: '后宫', icon: 'harem' },
      { name: 'reseed', label: '辅种', icon: 'seeding' },
      { name: 'assessment', label: '考核', icon: 'assessment' },
      { name: 'freeleech', label: '站免', icon: 'freeleech' },
      { name: 'savept', label: '监护室', icon: 'savept' },
      { name: 'brushflow', label: '刷流', icon: 'brushflow' },
      { name: 'contract', label: '保种契约', icon: 'contract' },
    ],
  },
  {
    key: 'infra',
    title: '基础设施',
    items: [
      { name: 'downloaders', label: '下载器', icon: 'downloaders' },
      { name: 'mediaserver', label: '媒体服务器', icon: 'mediaserver' },
      { name: 'files', label: '文件', icon: 'files' },
      { name: 'nas-server', label: 'NAS', icon: 'server' },
      { name: 'ops-center', label: '运维', icon: 'server' },
    ],
  },
  {
    key: 'automation',
    title: '自动化',
    items: [
      { name: 'workflow', label: '工作流', icon: 'workflow' },
      { name: 'plugins', label: '插件', icon: 'plugin' },
    ],
  },
]

// P2 移出侧栏但仍可 ⌘K / 深链 / 收藏：全局页与配置页保留在 paletteOnly。
export const paletteOnly: (NavItem & { group: string })[] = [
  { name: 'rss', label: 'RSS', icon: 'rss', group: '内容获取' },
  { name: 'transfer', label: '整理', icon: 'files', group: '内容获取' },
  { name: 'filter-rules', label: '过滤规则', icon: 'settings', group: '内容获取' },
  { name: 'cloud', label: '云盘·STRM', icon: 'cloud', group: '基础设施' },
  { name: 'subtitle', label: '字幕', icon: 'subtitle', group: '基础设施' },
  { name: 'trakt', label: 'Trakt', icon: 'trakt', group: '基础设施' },
  { name: 'mediaquality', label: '媒体质量', icon: 'mediaquality', group: '基础设施' },
  { name: 'backup', label: '备份', icon: 'backup', group: '基础设施' },
  { name: 'doctor', label: '诊断', icon: 'stethoscope', group: '基础设施' },
]

export const navDesc: Record<string, string> = {
  reseed: '辅种 · 跨站辅助种子',
  transfer: '整理 · 下载完成自动识别重命名硬链刮削入库',
  'filter-rules': '过滤规则 · 规则组表达式对种子分级筛选',
  assessment: '考核 · 保级 / 升级条件',
  freeleech: '站免 · 全站免费促销',
  harem: '后宫 · 邀请管理',
  'medal-wall': '勋章墙 · 勋章收集',
  entertainment: '娱乐 · 站内小游戏',
  savept: '监护室 · 站点健康监控',
  brushflow: '刷流 · 自动选种做种刷量',
  contract: '保种契约 · 分享率/做种/上传下限告警',
  mediaserver: '媒体服务器 · Emby/Jellyfin/Plex 库扫描与封面',
  cloud: '云盘·STRM · 115/夸克/123 浏览 + STRM 生成 + 订阅追更',
  subtitle: '字幕 · ASSRT/OpenSubtitles/SubHD/字幕库 聚合搜索下载',
  trakt: 'Trakt · WatchList 自动同步为本地订阅',
  plugins: '插件 · 事件扩展 / 动作执行',
  recommend: '推荐 · 热门 / 口碑 / 个性化',
  subscriptions: '订阅 · 追剧自动推送',
  sites: '站点 · Cookie / 连通 / 用户数据',
  rss: 'RSS · 资源源订阅',
  downloaders: '下载器 · qB/TR/Deluge/Aria2',
  files: '文件 · 本地/NAS/云盘统一浏览',
  'nas-server': 'NAS · 设备与服务监控',
  'ops-center': '运维 · 系统监控/Docker/文件/数据库',
  mediaquality: '媒体质量 · 多评分聚合 + 低分保护',
  backup: '备份 · 配置导出/导入',
  doctor: '诊断 · 健康自检',
  workflow: '工作流 · 可视化自动化',
  messages: '消息 · 站内信聚合',
  announcements: '公告 · 站点公告聚合',
  'site-requests': '求种 · 站点求种请求聚合',
  notices: '通知中心 · 站点/系统/考核/备份提醒聚合',
}

export const navAliases: Record<string, string[]> = {
  brushflow: ['刷量', '刷上传', 'auto brush', '刷ratio'],
  assessment: ['保级', '升级', '考核进度', 'HR考核'],
  savept: ['HR', 'Hit and Run', '健康', '监护'],
  contract: ['保种', '做种契约', '分享率告警', 'HR契约'],
  freeleech: ['FL', '免费', '全站免流', 'free'],
  reseed: ['cross-seed', '辅种匹配', '跨站辅种'],
  harem: ['邀请', '邀请树', 'invite'],
  'medal-wall': ['勋章', 'badge', 'medal'],
  entertainment: ['小游戏', '游戏', '抽奖'],
  downloaders: ['qb', 'qBittorrent', 'transmission', '下载'],
  mediaserver: ['Emby', 'Jellyfin', 'Plex', '媒体库'],
  cloud: ['115', '夸克', '123', 'STRM', '网盘'],
  transfer: ['硬链', '整理入库', '刮削'],
  doctor: ['自检', '诊断', 'health'],
  backup: ['导出', '导入', '备份恢复'],
  sites: ['PT站', '站点管理', 'cookie'],
  assistant: ['AI', '助手', 'chat'],
  search: ['搜种', '搜索', '种子', '影视'],
  settings: ['设置', '配置', '系统设置'],
}
