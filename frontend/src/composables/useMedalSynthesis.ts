// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { ref, computed, type Ref } from 'vue'
import {
  entertainmentApi,
  type GameStats,
  type SynthesisTarget,
  type GameRecord,
  type PlayResult,
} from '@/api/entertainment'
import { entFail } from '@/utils/entFail'

export function useMedalSynthesis(siteId: Ref<number>) {
  const targets = ref<SynthesisTarget[]>([])
  const stats = ref<GameStats | null>(null)
  const selectedId = ref<number | null>(null)
  const drawing = ref(0) // 正在抽取的次数（0=空闲）
  const synthesizing = ref(false)
  const lastDraw = ref<{ count: number; frags: { name: string; count: number }[]; msg: string } | null>(null)
  const showHistory = ref(false)
  const history = ref<GameRecord[]>([])
  const historyLoading = ref(false)

  const selected = computed(() => targets.value.find((t) => t.id === selectedId.value) || null)
  const progressPct = computed(() => {
    if (!selected.value || selected.value.need === 0) return 0
    return Math.min(100, Math.round((selected.value.got / selected.value.need) * 100))
  })

  async function loadAll() {
    try {
      const [ts, st] = await Promise.all([
        entertainmentApi.syntheses(siteId.value, 'medal_synthesis').catch(entFail('syntheses', [])),
        entertainmentApi.stats(siteId.value, 'medal_synthesis').catch(entFail('stats', null)),
      ])
      targets.value = ts
      stats.value = st
      if (selectedId.value === null || !ts.find((t) => t.id === selectedId.value)) {
        selectedId.value = ts[0]?.id ?? null
      }
    } catch {
      /* 静默 */
    }
  }

  function parseFrags(prize?: string) {
    if (!prize) return []
    return prize
      .split(/[、,]/)
      .map((s) => {
        const m = s.trim().match(/^(.+?)×(\d+)$/)
        return m ? { name: m[1], count: Number(m[2]) } : { name: s.trim(), count: 1 }
      })
      .filter((f) => f.name)
  }

  async function draw(count: number) {
    if (!selected.value || drawing.value) return
    drawing.value = count
    lastDraw.value = null
    try {
      const res: PlayResult = await entertainmentApi.draw(siteId.value, 'medal_synthesis', selected.value.id, count)
      if (res.success) {
        lastDraw.value = { count, frags: parseFrags(res.prize), msg: res.message || '' }
        ElMessage.success(res.message || `${count} 连抽取完成`)
      } else {
        ElMessage.warning(res.message || '抽取失败')
      }
      await loadAll() // 刷新碎片库存 + 火花
      if (showHistory.value) {
        entertainmentApi
          .history(siteId.value, 'medal_synthesis')
          .then((h) => (history.value = h))
          .catch(() => {})
      }
    } catch (e: unknown) {
      const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
      ElMessage.error(msg || '请求失败')
    } finally {
      drawing.value = 0
    }
  }

  async function synthesize() {
    if (!selected.value || synthesizing.value) return
    synthesizing.value = true
    try {
      const res = await entertainmentApi.synthesize(siteId.value, 'medal_synthesis', selected.value.id)
      if (res.success) ElMessage.success(res.prize ? `合成成功：${res.prize}` : '合成成功')
      else ElMessage.warning(res.message || '合成失败')
      await loadAll()
    } catch (e: unknown) {
      const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
      ElMessage.error(msg || '请求失败')
    } finally {
      synthesizing.value = false
    }
  }

  async function toggleHistory() {
    showHistory.value = !showHistory.value
    if (showHistory.value && !history.value.length) {
      historyLoading.value = true
      try {
        history.value = await entertainmentApi.history(siteId.value, 'medal_synthesis')
      } catch {
        ElMessage.warning('记录加载失败')
      } finally {
        historyLoading.value = false
      }
    }
  }

  return {
    targets,
    stats,
    selectedId,
    drawing,
    synthesizing,
    lastDraw,
    showHistory,
    history,
    historyLoading,
    selected,
    progressPct,
    loadAll,
    draw,
    synthesize,
    toggleHistory,
  }
}

export function fmtNum(n: number) {
  return Math.round(n).toLocaleString('zh-CN')
}
