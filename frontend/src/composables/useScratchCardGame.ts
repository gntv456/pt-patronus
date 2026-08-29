import { ref, watch, onMounted, onUnmounted, type Ref } from 'vue'
import {
  entertainmentApi,
  type GameStats,
  type BuyOption,
  type PrizeDef,
  type GameRecord,
  type PlayResult,
} from '@/api/entertainment'

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
        entertainmentApi.stats(siteId.value, 'cspt_scratch').catch(() => null),
        entertainmentApi.buyOptions(siteId.value, 'cspt_scratch').catch(() => []),
        entertainmentApi.prizes(siteId.value, 'cspt_scratch').catch(() => []),
        entertainmentApi.history(siteId.value, 'cspt_scratch').catch(() => []),
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
    stats.value = await entertainmentApi.stats(siteId.value, 'cspt_scratch').catch(() => stats.value)
  }

  async function play() {
    if (playing.value) return
    playing.value = true
    reveal.value = null
    try {
      const res = await entertainmentApi.play(siteId.value, 'cspt_scratch')
      reveal.value = res
      if (res.success) ElMessage.success(res.prize ? `获得 ${res.prize}` : '刮卡成功')
      else ElMessage.warning(res.message || '刮卡失败')
      await refreshStats()
      history.value = await entertainmentApi.history(siteId.value, 'cspt_scratch').catch(() => history.value)
    } catch (e: unknown) {
      const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
      ElMessage.error(msg || '请求失败')
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
      else ElMessage.warning(res.message || '购买失败')
      await refreshStats()
    } catch (e: unknown) {
      const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
      ElMessage.error(msg || '购买失败')
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
