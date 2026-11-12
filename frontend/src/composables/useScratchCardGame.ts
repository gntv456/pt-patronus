// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { ref, watch, onMounted, onUnmounted, type Ref } from 'vue'
import { i18n } from '@/i18n'
import {
  entertainmentApi,
  type GameStats,
  type BuyOption,
  type PrizeDef,
  type GameRecord,
  type PlayResult,
} from '@/api/entertainment'
import { entFail } from '@/utils/entFail'


const t = (k: string) => i18n.global.t(k)

export function useScratchCardGame(siteId: Ref<number>) {
  const stats = ref<GameStats | null>(null)
  const options = ref<BuyOption[]>([])
  const prizes = ref<PrizeDef[]>([])
  const history = ref<GameRecord[]>([])

  const playing = ref(false)
  const buying = ref(false)
  const autoPlaying = ref(false)
  const autoCount = ref(0)
  const reveal = ref<PlayResult | null>(null) // 当前刮开的卡片结果
  let autoTimer: ReturnType<typeof setInterval> | null = null

  async function loadAll() {
    try {
      const [st, opt, pz, hi] = await Promise.all([
        entertainmentApi.stats(siteId.value, 'cspt_scratch').catch(entFail('stats', null)),
        entertainmentApi.buyOptions(siteId.value, 'cspt_scratch').catch(entFail('buyOptions', [])),
        entertainmentApi.prizes(siteId.value, 'cspt_scratch').catch(entFail('prizes', [])),
        entertainmentApi.history(siteId.value, 'cspt_scratch').catch(entFail('history', [])),
      ])
      stats.value = st
      options.value = opt
      prizes.value = pz
      history.value = hi
    } catch {
      /* 静默 */
    }
  }

  async function refreshStats() {
    stats.value = await entertainmentApi.stats(siteId.value, 'cspt_scratch').catch(entFail('stats', stats.value))
  }

  async function play() {
    if (playing.value) return
    playing.value = true
    reveal.value = null
    try {
      const res = await entertainmentApi.play(siteId.value, 'cspt_scratch')
      reveal.value = res
      if (res.success) ElMessage.success(res.prize ? `获得 ${res.prize}` : t('opusescratchcardgame.k1'))
      else ElMessage.warning(res.message || t('opusescratchcardgame.k2'))
      await refreshStats()
      history.value = await entertainmentApi.history(siteId.value, 'cspt_scratch').catch(entFail('history', history.value))
    } catch (e: unknown) {
      const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
      ElMessage.error(msg || t('common.requestFailed'))
    } finally {
      playing.value = false
    }
  }

  function toggleAuto() {
    if (autoPlaying.value) stopAuto()
    else startAuto()
  }
  function startAuto() {
    if (autoPlaying.value) return
    autoPlaying.value = true
    autoCount.value = 0
    autoTimer = setInterval(async () => {
      if ((stats.value?.card_number ?? 0) <= 0 || playing.value) {
        stopAuto()
        return
      }
      await play()
      autoCount.value++
    }, 700)
  }
  function stopAuto() {
    autoPlaying.value = false
    if (autoTimer) clearInterval(autoTimer)
    autoTimer = null
  }

  async function buy(opt: BuyOption) {
    if (buying.value) return
    buying.value = true
    try {
      const res = await entertainmentApi.buy(siteId.value, 'cspt_scratch', opt.type)
      if (res.success) ElMessage.success(res.message || `购买 ${opt.buy_count} 张`)
      else ElMessage.warning(res.message || t('opusescratchcardgame.k3'))
      await refreshStats()
    } catch (e: unknown) {
      const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
      ElMessage.error(msg || t('opusescratchcardgame.k3'))
    } finally {
      buying.value = false
    }
  }

  onMounted(loadAll)
  onUnmounted(stopAuto)
  watch(siteId, loadAll)

  return {
    stats,
    options,
    prizes,
    history,
    playing,
    buying,
    autoPlaying,
    autoCount,
    reveal,
    loadAll,
    refreshStats,
    play,
    toggleAuto,
    startAuto,
    stopAuto,
    buy,
  }
}

export function fmt(n: number) {
  return Math.round(n).toLocaleString('zh-CN')
}
