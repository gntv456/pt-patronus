import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'

const state = vi.hoisted(() => ({ alertsFail: true }))

vi.mock('@/api/monitor', () => ({
  monitorApi: {
    list: vi.fn(async () => [{ id: 1, name: 'nas-01' }]),
    alerts: vi.fn(async () => {
      if (state.alertsFail) throw new Error('monitor alerts 500')
      // 契约：alerts() 回分页页（含 total/truncated），不再只回数组
      return { alerts: [{ id: 7, level: 'danger', title: '磁盘掉线' }], total: 1, limit: 200, truncated: false }
    }),
    metrics: vi.fn(async () => []),
  },
  credentialApi: { list: vi.fn(async () => []) },
  opsConfigApi: { list: vi.fn(async () => []) },
  commandRunApi: { list: vi.fn(async () => []) },
  parseMetrics: (s: unknown) => s,
}))

import { useNasLoader } from './useNasLoader'

function harness() {
  return useNasLoader({
    isAdmin: false,
    targets: ref([]) as never,
    alerts: ref([]) as never,
    credentials: ref([]) as never,
    opsConfigs: ref([]) as never,
    commandRuns: ref([]) as never,
    latestMap: ref({}) as never,
    t: (k: string) => k,
  })
}

// 设备列表失败会走整页错误态，但「未读告警」以前被 .catch(() => []) 抹平：
// 告警按钮不亮红也不挂数字，看着像「没有未读告警」，而设备可能正挂着。
describe('useNasLoader · 告警读取失败要留痕', () => {
  beforeEach(() => {
    state.alertsFail = true
  })

  it('告警接口失败：alertsFailed 置真、error 不误报整页故障', async () => {
    const targets = ref<never[]>([])
    const alerts = ref<never[]>([])
    const l = useNasLoader({
      isAdmin: false,
      targets,
      alerts,
      credentials: ref([]) as never,
      opsConfigs: ref([]) as never,
      commandRuns: ref([]) as never,
      latestMap: ref({}) as never,
      t: (k: string) => k,
    })
    await l.loadAll()
    expect(l.alertsFailed.value).toBe(true)
    expect(alerts.value).toEqual([])
    expect(l.error.value).toBe('')
    expect(targets.value.length).toBe(1)
  })

  it('正控：告警读得到时 alertsFailed 保持假', async () => {
    state.alertsFail = false
    const alerts = ref<never[]>([])
    const l = useNasLoader({
      isAdmin: false,
      targets: ref([]) as never,
      alerts,
      credentials: ref([]) as never,
      opsConfigs: ref([]) as never,
      commandRuns: ref([]) as never,
      latestMap: ref({}) as never,
      t: (k: string) => k,
    })
    await l.loadAll()
    expect(l.alertsFailed.value).toBe(false)
    expect(alerts.value.length).toBe(1)
  })

  it('harness 复用检查：恢复后再次加载会清掉上一次的失败位', async () => {
    const l = harness()
    await l.loadAll()
    expect(l.alertsFailed.value).toBe(true)
    state.alertsFail = false
    await l.loadAll()
    expect(l.alertsFailed.value).toBe(false)
  })
})
