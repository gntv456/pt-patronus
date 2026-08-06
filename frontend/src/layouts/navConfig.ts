// 导航配置数据：侧栏分组、palette-only 项、描述、别名

export interface NavItem {
  name: string
  label: string
  icon: string
  from: string
  to: string
}
export interface NavGroup {
  title: string
  items: NavItem[]
}

// P2: 侧栏收敛到定稿主菜单；移出项保留在 paletteOnly（⌘K/收藏/深链）。
export const navGroups: NavGroup[] = [
  {
    title: '概览',
    items: [
      { name: 'dashboard', label: '仪表盘', icon: 'dashboard', from: '#6c5ce7', to: '#4285f4' },
      { name: 'recommend', label: '推荐', icon: 'recommend', from: '#f7971e', to: '#ff6a00' },
      { name: 'subscriptions', label: '订阅', icon: 'subscribe', from: '#e100ff', to: '#ff6a00' },
      { name: 'sites', label: '站点', icon: 'sites', from: '#4285f4', to: '#22d3ee' },
    ],
  },
  {
    title: '动态',
    items: [
      { name: 'messages', label: '消息', icon: 'message', from: '#2193b0', to: '#6dd5ed' },
      { name: 'announcements', label: '公告', icon: 'announcement', from: '#f7971e', to: '#ffd200' },
    ],
  },
  {
    title: '站点活动',
    items: [
      { name: 'entertainment', label: '娱乐', icon: 'entertainment', from: '#7f00ff', to: '#e100ff' },
      { name: 'medal-wall', label: '勋章墙', icon: 'medalwall', from: '#f39c12', to: '#e100ff' },
      { name: 'harem', label: '后宫', icon: 'harem', from: '#e100ff', to: '#7f00ff' },
      { name: 'reseed', label: '辅种', icon: 'seeding', from: '#7f00ff', to: '#38ef7d' },
      { name: 'assessment', label: '考核', icon: 'assessment', from: '#0f9b0f', to: '#43cea2' },
      { name: 'freeleech', label: '站免', icon: 'freeleech', from: '#11998e', to: '#38ef7d' },
      { name: 'savept', label: '监护室', icon: 'savept', from: '#e74c3c', to: '#f39c12' },
      { name: 'brushflow', label: '刷流', icon: 'brushflow', from: '#f7971e', to: '#ff6a00' },
      { name: 'contract', label: '保种契约', icon: 'contract', from: '#6c5ce7', to: '#4285f4' },
    ],
  },
  {
    title: '基础设施',
    items: [
      { name: 'downloaders', label: '下载器', icon: 'downloaders', from: '#11998e', to: '#38ef7d' },
      { name: 'mediaserver', label: '媒体服务器', icon: 'mediaserver', from: '#52b788', to: '#2d6a4f' },
      { name: 'files', label: '文件', icon: 'files', from: '#f7971e', to: '#ffd200' },
      { name: 'nas-server', label: 'NAS', icon: 'server', from: '#0f2027', to: '#2c5364' },
      { name: 'ops-center', label: '运维', icon: 'server', from: '#11998e', to: '#38ef7d' },
    ],
  },
  {
    title: '自动化',
    items: [
      { name: 'workflow', label: '工作流', icon: 'workflow', from: '#6c5ce7', to: '#00b894' },
      { name: 'plugins', label: '插件', icon: 'plugin', from: '#0ea5e9', to: '#22c55e' },
    ],
  },
]

// P2 移出侧栏但仍可 ⌘K / 深链 / 收藏：全局页与配置页保留在 paletteOnly。
export const paletteOnly: (NavItem & { group: string })[] = [
  { name: 'rss', label: 'RSS', icon: 'rss', from: '#f7971e', to: '#ff6a00', group: '内容获取' },
  { name: 'transfer', label: '整理', icon: 'files', from: '#6c5ce7', to: '#00b894', group: '内容获取' },
  { name: 'filter-rules', label: '过滤规则', icon: 'settings', from: '#6c5ce7', to: '#4285f4', group: '内容获取' },
  { name: 'cloud', label: '云盘·STRM', icon: 'cloud', from: '#4cc9f0', to: '#4361ee', group: '基础设施' },
  { name: 'subtitle', label: '字幕', icon: 'subtitle', from: '#4361ee', to: '#7209b7', group: '基础设施' },
  { name: 'trakt', label: 'Trakt', icon: 'trakt', from: '#b5179e', to: '#f72585', group: '基础设施' },
  { name: 'mediaquality', label: '媒体质量', icon: 'mediaquality', from: '#f12711', to: '#f5af19', group: '基础设施' },
  { name: 'backup', label: '备份', icon: 'backup', from: '#536976', to: '#2c3e50', group: '基础设施' },
  { name: 'doctor', label: '诊断', icon: 'stethoscope', from: '#0f2027', to: '#ef4444', group: '基础设施' },
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
