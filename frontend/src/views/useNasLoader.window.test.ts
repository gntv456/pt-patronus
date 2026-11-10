// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// @vitest-environment jsdom
// 运维命令批次台账 + 告警清单的取数账面接线：后端已把 limit/total/truncated 一起回，
// 若 composable 把字段丢掉，NasServer 面板就会把「最近 20 次」当成这台机器的全部执行历史。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'

const state = vi.hoisted(() => ({
  runs: {
    runs: [] as Array<Record<string, unknown>>,
    total: undefined as number | undefined,
    limit: 20,
    truncated: false,
  },
  alerts: {
    alerts: [] as Array<Record<string, unknown>>,
    total: 0,
    limit: 200,
    truncated: false,
  },
  failRuns: false,
}))

vi.mock('@/api/monitor', () => ({
  monitorApi: {
    list: vi.fn(async () => [{ id: 1, name: 'h', type: 'host' }]),
    alerts: vi.fn(async () => state.alerts),
    metrics: vi.fn(async () => ({ snapshots: [], limit: 1, truncated: false })),
  },
  commandRunApi: {
    listPage: vi.fn(async () => {
      if (state.failRuns) throw new Error('boom')
      return state.runs
    }),
  },
  credentialApi: { list: vi.fn(async () => []) },
  opsConfigApi: { list: vi.fn(async () => []) },
  parseMetrics: vi.fn(() => null),
}))

const { useNasLoader } = await import('@/views/useNasLoader')

function loader() {
  return useNasLoader({
    isAdmin: true,
    targets: ref([]) as never,
    alerts: ref([]) as never,
    credentials: ref([]) as never,
    opsConfigs: ref([]) as never,
    commandRuns: ref([]) as never,
    latestMap: ref({}) as never,
    t: (k: string) => k,
  })
}

function rows(n: number) {
  return Array.from({ length: n }, (_, i) => ({ id: i + 1 }))
}

describe('useNasLoader 批次台账与告警窗口账面', () => {
  beforeEach(() => {
    state.failRuns = false
    state.runs = { runs: rows(20), total: 137, limit: 20, truncated: true }
    state.alerts = { alerts: [], total: 0, limit: 200, truncated: false }
  })

  it('台账被单页上限截断：truncated/total 必须落到界面读得到的 ref', async () => {
    const st = loader()
    await st.loadAll()
    expect(st.runsTruncated.value).toBe(true)
    expect(st.runsTotal.value).toBe(137)
  })

  it('正控：一次拿全时不留截断痕迹', async () => {
    state.runs = { runs: rows(1), total: 1, limit: 20, truncated: false }
    const st = loader()
    await st.loadAll()
    expect(st.runsTruncated.value).toBe(false)
    expect(st.runsTotal.value).toBe(1)
  })

  it('台账读取失败：保守按「可能被截断」，总数留空不编 0', async () => {
    state.failRuns = true
    const st = loader()
    await st.loadAll()
    expect(st.runsTruncated.value).toBe(true)
    expect(st.runsTotal.value).toBeUndefined()
  })

  it('告警清单同样把 total/truncated 接回来（角标不能只数这一页）', async () => {
    state.alerts = { alerts: rows(200), total: 412, limit: 200, truncated: true }
    const st = loader()
    await st.loadAll()
    expect(st.alertsTruncated.value).toBe(true)
    expect(st.alertsTotal.value).toBe(412)
  })
})
