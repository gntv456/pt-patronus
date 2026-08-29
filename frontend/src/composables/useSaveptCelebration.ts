// PT监护室站庆彩带：canvas-confetti 动态加载 + 生命周期管理。
// 从 Savept.vue 抽出；依赖分组后的站点列表判断哪些卡片今日站庆。
import { computed, nextTick, ref, watch, onMounted, onBeforeUnmount } from 'vue'
import { isTodayAnniversaryText } from '@/utils/savept'
import type { SaveptSiteCard } from '@/api/savept'

type ConfettiFn = ((opts: Record<string, unknown>) => unknown) & { reset?: () => void }
type ConfettiFactory = (canvas: HTMLCanvasElement, opts: Record<string, unknown>) => ConfettiFn
type ConfettiMain = ConfettiFn & { create?: ConfettiFactory }
type ConfettiModule = { default?: ConfettiMain; create?: ConfettiFactory } & Partial<ConfettiMain>

const celebrationCanvasRefs = new Map<string, HTMLCanvasElement>()
const celebrationInstances = new Map<string, ConfettiFn>()
const celebrationTimers = new Map<string, ReturnType<typeof setTimeout>>()
let confettiFactory: ConfettiFactory | null = null
let confettiLoader: Promise<ConfettiFactory | null> | null = null
const prefersReducedMotion = ref(false)
let reducedMotionMQ: MediaQueryList | null = null

function isCompact() {
  return typeof window !== 'undefined' && window.innerWidth <= 768
}
function loopDelay() {
  return isCompact() ? 3600 : 2800
}
async function ensureFactory(): Promise<ConfettiFactory | null> {
  if (confettiFactory) return confettiFactory
  if (!confettiLoader) {
    confettiLoader = import('canvas-confetti')
      .then((m) => {
        const mod = m as unknown as ConfettiModule
        const main = (mod.default || mod) as ConfettiMain
        const create = main.create || mod.create
        if (!create) throw new Error('canvas-confetti create() 不可用')
        confettiFactory = create
        return create
      })
      .catch((e) => {
        console.error('[savept] 加载 canvas-confetti 失败', e)
        confettiLoader = null
        return null
      })
  }
  return confettiLoader
}
async function ensureInstance(key: string) {
  if (celebrationInstances.has(key)) return celebrationInstances.get(key)!
  const canvas = celebrationCanvasRefs.get(key)
  if (!canvas || prefersReducedMotion.value) return null
  const factory = await ensureFactory()
  if (!factory || !celebrationCanvasRefs.has(key)) return null
  const instance = factory(canvas, { resize: true, useWorker: false })
  celebrationInstances.set(key, instance)
  return instance
}
function burstOpts(side: 'left' | 'right') {
  const c = isCompact()
  return {
    particleCount: c ? (side === 'left' ? 10 : 8) : side === 'left' ? 18 : 14,
    angle: side === 'left' ? 52 : 128,
    spread: c ? 44 : 60,
    startVelocity: c ? 18 : 24,
    scalar: c ? 0.72 : 0.92,
    ticks: c ? 70 : 95,
    gravity: c ? 1.08 : 0.96,
    drift: side === 'left' ? 0.08 : -0.08,
    origin: side === 'left' ? { x: c ? 0.08 : 0.06, y: c ? 0.94 : 0.92 } : { x: c ? 0.92 : 0.94, y: c ? 0.94 : 0.92 },
    colors: ['#f59e0b', '#facc15', '#fb7185', '#22c55e', '#38bdf8'],
  }
}
function stopLoop(key: string) {
  const t = celebrationTimers.get(key)
  if (t) clearTimeout(t)
  celebrationTimers.delete(key)
}
function destroyInstance(key: string) {
  stopLoop(key)
  celebrationInstances.get(key)?.reset?.()
  celebrationInstances.delete(key)
}
function queueLoop(key: string) {
  stopLoop(key)
  celebrationTimers.set(
    key,
    setTimeout(() => startLoop(key), loopDelay()),
  )
}
async function startLoop(key: string) {
  stopLoop(key)
  if (prefersReducedMotion.value || document.visibilityState === 'hidden') return
  const confetti = await ensureInstance(key)
  if (!confetti || !celebrationCanvasRefs.has(key)) return
  confetti(burstOpts('left'))
  confetti(burstOpts('right'))
  if (!prefersReducedMotion.value && document.visibilityState === 'visible') queueLoop(key)
}
function syncCelebration() {
  if (typeof window === 'undefined') return
  const active = new Set(activeAnniversaryKeys.value.filter((k) => celebrationCanvasRefs.has(k)))
  for (const k of celebrationTimers.keys()) if (!active.has(k)) stopLoop(k)
  for (const k of celebrationInstances.keys()) if (!active.has(k)) destroyInstance(k)
  if (prefersReducedMotion.value || document.visibilityState === 'hidden') return
  for (const k of active) if (!celebrationTimers.has(k)) startLoop(k)
}
function handleReducedMotionChange(e: MediaQueryListEvent) {
  prefersReducedMotion.value = !!e.matches
  syncCelebration()
}
function handleVisibilityChange() {
  syncCelebration()
}

// 由调用方提供的分组数据（模块级缓存 computed，避免每次调用重建）
let activeAnniversaryKeys = computed((): string[] => [])

function setCelebrationRef(key: string, el: Element | null) {
  if (el) celebrationCanvasRefs.set(key, el as HTMLCanvasElement)
  else celebrationCanvasRefs.delete(key)
}

export function useSaveptCelebration(
  groupedSites: () => { year: string; items: SaveptSiteCard[] }[],
  isYearGroupCollapsed: (year: string) => boolean,
) {
  // computed 赋给 ref 变量本身（activeAnniversaryKeys = computed(...)）而非 .value——
  // .value 是 string[] 不吃 ComputedRef；改为持有 computed 并在读取处解包。
  activeAnniversaryKeys = computed(() => {
    const keys: string[] = []
    for (const g of groupedSites()) {
      if (isYearGroupCollapsed(g.year)) continue
      for (const card of g.items) {
        if (isTodayAnniversaryText(card.anniversary_text)) keys.push(`${g.year}-${card.name || card.url}`)
      }
    }
    return keys
  })

  watch(
    activeAnniversaryKeys,
    async () => {
      await nextTick()
      syncCelebration()
    },
    { flush: 'post' },
  )

  onMounted(() => {
    reducedMotionMQ = window.matchMedia('(prefers-reduced-motion: reduce)')
    prefersReducedMotion.value = reducedMotionMQ.matches
    reducedMotionMQ.addEventListener('change', handleReducedMotionChange)
    document.addEventListener('visibilitychange', handleVisibilityChange)
    window.addEventListener('resize', syncCelebration, { passive: true })
  })

  onBeforeUnmount(() => {
    document.removeEventListener('visibilitychange', handleVisibilityChange)
    reducedMotionMQ?.removeEventListener('change', handleReducedMotionChange)
    for (const k of celebrationTimers.keys()) stopLoop(k)
    for (const k of celebrationInstances.keys()) destroyInstance(k)
    celebrationCanvasRefs.clear()
    window.removeEventListener('resize', syncCelebration)
  })

  return { setCelebrationRef, syncCelebration }
}
