import { ref, computed, type Ref } from 'vue'
import { entertainmentApi, type GameStats, type PlayResult, type GameRecord } from '@/api/entertainment'

// 9 个奖品（源码 jgg/magic_grid_config.php 固定配置；id 1-8 占外圈，id9 归到 VIP1天格）。
export interface Prize {
  id: number
  name: string
  icon: string
  color: string
}

export const prizes: Prize[] = [
  { id: 1, name: '勋章', icon: '🎊', color: '#4CAF50' },
  { id: 2, name: '临时邀请码', icon: '⏱️', color: '#2196F3' },
  { id: 3, name: '1GB上传量', icon: '💾', color: '#9C27B0' },
  { id: 4, name: '10GB上传量', icon: '💾', color: '#FF9800' },
  { id: 5, name: '10GB下载量', icon: '📥', color: '#00BCD4' },
  { id: 6, name: 'VIP(1天)', icon: '👑', color: '#FF5722' },
  { id: 7, name: '邀请码', icon: '✉️', color: '#795548' },
  { id: 8, name: '谢谢参与', icon: '⭐', color: '#607D8B' },
  { id: 9, name: 'VIP(7天)', icon: '👑', color: '#FFD700' },
]

// 外圈顺时针（id 1-8 → 索引 0-7）；网格行主序布局，-1 = 中心按钮。
export const clockwise = prizes.slice(0, 8) // id 1-8
export const layout = [
  [0, 1, 2],
  [7, -1, 3],
  [6, 5, 4],
]

// 奖品 id → 外圈位置（0-7）。id9 归到 pos5（VIP1天格）。
export function mapIdToPos(id: number): number {
  if (id >= 1 && id <= 8) return id - 1
  if (id === 9) return 5
  return 7
}

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms))

export function useJggGame(siteId: Ref<number>) {
  const stats = ref<GameStats | null>(null)
  const spinning = ref(false)
  const highlight = ref(-1) // 当前高亮的外圈索引；-1 无
  const result = ref<PlayResult | null>(null)
  // 连抽（10/20/50/100）与抽奖历史
  const bulk = ref<PlayResult | null>(null)
  const bulkCount = ref(0)
  const bulkSpinning = ref(false)
  const records = ref<GameRecord[]>([])

  // 连抽结果按奖品名分组统计（等价于站点 prize_stats）。
  const bulkStats = computed(() => {
    const map = new Map<string, number>()
    for (const p of bulk.value?.prizes ?? []) {
      map.set(p.prize_name, (map.get(p.prize_name) ?? 0) + 1)
    }
    return [...map.entries()].map(([name, count]) => ({ name, count })).sort((a, b) => b.count - a.count)
  })

  async function refreshStats() {
    stats.value = await entertainmentApi.stats(siteId.value, 'jgg').catch(() => stats.value)
  }

  async function refreshHistory() {
    records.value = await entertainmentApi.history(siteId.value, 'jgg').catch(() => records.value)
  }

  async function spin() {
    if (spinning.value || bulkSpinning.value) return
    spinning.value = true
    result.value = null
    bulk.value = null
    highlight.value = -1
    try {
      const res = await entertainmentApi.play(siteId.value, 'jgg')
      if (res.success) {
        // 转 3 圈 + 到目标格，逐步减速（对齐 Kotlin 节奏）。
        const target = mapIdToPos(res.prize_id || 0)
        const total = 3 * 8 + target
        let delay = 70
        for (let i = 0; i <= total; i++) {
          highlight.value = i % 8
          await sleep(delay)
          delay = Math.min(delay + 8, 280)
        }
        result.value = res
        const win = res.prize && res.prize !== '谢谢参与'
        if (win) ElMessage.success(`🎉 获得 ${res.prize}`)
        else ElMessage.info(res.message || '谢谢参与')
      } else {
        result.value = res
        ElMessage.warning(res.message || '抽奖失败')
      }
      await Promise.all([refreshStats(), refreshHistory()])
    } catch (e: unknown) {
      const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
      ElMessage.error(msg || '请求失败')
    } finally {
      spinning.value = false
    }
  }

  // 连抽：站点 /jgg/magic_grid_bulk.php 一次返回 N 份结果，无单一中奖格，故仅做装饰性流转反馈。
  async function drawMulti(count: number) {
    if (spinning.value || bulkSpinning.value) return
    bulkSpinning.value = true
    bulk.value = null
    result.value = null
    highlight.value = -1
    try {
      let delay = 55
      for (let i = 0; i < 18; i++) {
        highlight.value = i % 8
        await sleep(delay)
      }
      highlight.value = -1
      const res = await entertainmentApi.playMulti(siteId.value, 'jgg', count)
      if (res.success) {
        bulk.value = res
        bulkCount.value = count
        const wins = (res.prizes || []).filter((p) => p.is_win).length
        ElMessage.success(`🎉 ${count}连抽完成，中奖 ${wins}/${count}`)
      } else {
        ElMessage.warning(res.message || '连抽失败')
      }
      await Promise.all([refreshStats(), refreshHistory()])
    } catch (e: unknown) {
      const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
      ElMessage.error(msg || '请求失败')
    } finally {
      bulkSpinning.value = false
    }
  }

  return {
    stats,
    spinning,
    highlight,
    result,
    bulk,
    bulkCount,
    bulkSpinning,
    records,
    bulkStats,
    refreshStats,
    refreshHistory,
    spin,
    drawMulti,
  }
}
