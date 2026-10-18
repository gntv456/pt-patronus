// @vitest-environment jsdom
// 接线回归：告警被单页上限截断时，加载器要把 truncated/total 暴露成状态，
// 供 NasServer 头部按钮说出来——否则按钮上的数字就是「全部未读」。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'

const api = vi.hoisted(() => ({
  list: vi.fn(),
  alerts: vi.fn(),
  metrics: vi.fn(),
}))

vi.mock('@/api/monitor', () => ({
  monitorApi: api,
  credentialApi: { list: vi.fn(() => Promise.resolve([])) },
  opsConfigApi: { list: vi.fn(() => Promise.resolve([])) },
  commandRunApi: { list: vi.fn(() => Promise.resolve([])) },
  parseMetrics: (s: unknown) => s,
}))

import { useNasLoader } from './useNasLoader'

function target(id: number) {
  return { id, name: `t${id}`, type: 'host', enabled: true } as never
}

function build() {
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

describe('useNasLoader 告警截断留痕', () => {
  beforeEach(() => {
    api.list.mockReset()
    api.alerts.mockReset()
    api.metrics.mockReset()
    api.list.mockResolvedValue([target(1)])
    api.metrics.mockResolvedValue([])
  })

  it('被截断时 alertsTruncated=true 且带上真实总数', async () => {
    const alerts = Array.from({ length: 200 }, (_, i) => ({ id: i }))
    api.alerts.mockResolvedValue({ alerts, total: 203, limit: 200, truncated: true })
    const s = build()
    await s.loadAll()
    expect(s.alertsTruncated.value).toBe(true)
    expect(s.alertsTotal.value).toBe(203)
  })

  it('未截断时不报错态（别把「都显示了」也说成截断）', async () => {
    api.alerts.mockResolvedValue({ alerts: [{ id: 1 }], total: 1, limit: 200, truncated: false })
    const s = build()
    await s.loadAll()
    expect(s.alertsTruncated.value).toBe(false)
  })

  it('告警读失败仍是 alertsFailed 留痕，不留下上一次的截断态', async () => {
    api.alerts.mockRejectedValue(new Error('boom'))
    const s = build()
    await s.loadAll()
    expect(s.alertsFailed.value).toBe(true)
    expect(s.alertsTruncated.value).toBe(false)
    expect(s.alertsTotal.value).toBe(0)
  })
})
