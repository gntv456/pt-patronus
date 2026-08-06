import type { ArrKey, SortField } from '@/composables/useSearchFilter'

// 搜索筛选条的静态配置选项（维度/时间/排序字段）。
// 抽出为独立文件以收敛 SearchFilterBar.vue 体积。

export const SEARCH_DIMS: { key: ArrKey; label: string; icon: string }[] = [
  { key: 'site', label: '站点', icon: '🌐' },
  { key: 'promo', label: '促销', icon: '🎁' },
  { key: 'res', label: '分辨率', icon: '🖥' },
  { key: 'codec', label: '编码', icon: '🎬' },
  { key: 'src', label: '来源', icon: '📦' },
  { key: 'hdr', label: '特效', icon: '✨' },
]

export const TIME_OPTS: { key: 'today' | 'week' | 'month'; label: string }[] = [
  { key: 'today', label: '今天' },
  { key: 'week', label: '本周' },
  { key: 'month', label: '本月' },
]

export const SORT_FIELDS: { key: SortField; label: string }[] = [
  { key: 'seeders', label: '做种数' },
  { key: 'size', label: '大小' },
  { key: 'leechers', label: '下载数' },
  { key: 'site', label: '站点' },
]
