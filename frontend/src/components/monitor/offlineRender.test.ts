// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// @vitest-environment jsdom
// 渲染层回归：采集失败时后端会落一条 {"online":false} 的空指标快照（既定设计），
// 前端不能把它渲染成健康读数——
// ① MetricBar：无读数显示 "-"，不画 0.0% 绿条；
// ② MetricChart：null 断口分段绘制（离线时段不连成「跌到 0%」的假断崖），孤立单点画圆点；
// ③ useDeviceDetail.seriesOf：缺失指标 → null 断口（不补 0）；
// ④ DeviceCard：离线/仅 online 快照走 no-data 分支显示 last_error，而非 CPU 0.0%。
import { describe, expect, it, vi } from 'vitest'
import { createApp, ref } from 'vue'
import { createI18n } from 'vue-i18n'
import ElementPlus from 'element-plus'
import zhCN from '@/i18n/locales/zh-CN'
import type { MonitorTarget, Metrics } from '@/api/monitor'
import MetricBar from './MetricBar.vue'
import MetricChart from './MetricChart.vue'
import DeviceCard from './DeviceCard.vue'
import DeviceHostDetail from './DeviceHostDetail.vue'
import { useDeviceDetail } from './useDeviceDetail'

const state = vi.hoisted(() => ({
  snaps: [] as Array<{ online: boolean; metrics: string; snap_at: string }>,
  total: undefined as number | undefined,
  truncated: false,
}))

vi.mock('@/api/monitor', async (importOriginal) => {
  const orig = await importOriginal<typeof import('@/api/monitor')>()
  return {
    ...orig,
    monitorApi: {
      collect: vi.fn(async () => null),
      metrics: vi.fn(async () => ({
        snapshots: state.snaps,
        limit: 120,
        total: state.total,
        truncated: state.truncated,
      })),
    },
  }
})

function mountNode(comp: unknown, props: Record<string, unknown>): HTMLElement {
  const el = document.createElement('div')
  document.body.appendChild(el)
  const i18n = createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zhCN } })
  const app = createApp(comp as never, props)
  app.use(i18n)
  app.use(ElementPlus)
  app.mount(el)
  return el
}

function text(el: HTMLElement): string {
  return (el.textContent ?? '').replace(/\s+/g, ' ')
}

describe('MetricBar：无读数不画成 0.0%', () => {
  it('value=undefined 显示 "-"，不出现 0.0%', () => {
    const el = mountNode(MetricBar, { label: 'CPU' })
    const t = text(el)
    expect(t).toContain('-')
    expect(t).not.toContain('0.0%')
  })

  it('正控：value=45 正常渲染 45.0%', () => {
    const el = mountNode(MetricBar, { label: 'CPU', value: 45 })
    expect(text(el)).toContain('45.0%')
  })
})

describe('MetricChart：null 断口分段绘制', () => {
  it('中间断口画出两段线（两个 M），路径无 NaN', () => {
    const el = mountNode(MetricChart, { data: [40, 50, null, 60, 70] })
    const d = el.querySelector('path[stroke]')?.getAttribute('d') ?? ''
    expect((d.match(/M/g) ?? []).length).toBe(2)
    expect(d).not.toContain('NaN')
    expect(d).not.toContain('null')
  })

  it('正控：无断口是一条连续线（一个 M）', () => {
    const el = mountNode(MetricChart, { data: [30, 60, 90] })
    const d = el.querySelector('path[stroke]')?.getAttribute('d') ?? ''
    expect((d.match(/M/g) ?? []).length).toBe(1)
  })

  it('两侧皆断的孤立单点画圆点，不留空白图', () => {
    const el = mountNode(MetricChart, { data: [null, 50, null] })
    expect(el.querySelectorAll('circle').length).toBe(1)
    expect(text(el)).not.toContain('暂无数据')
  })

  it('全为断口时显示「暂无数据」', () => {
    const el = mountNode(MetricChart, { data: [null, null, null] })
    expect(text(el)).toContain('暂无数据')
    expect(el.querySelectorAll('circle').length).toBe(0)
  })
})

describe('useDeviceDetail.seriesOf：缺失指标 → null', () => {
  it('离线快照在序列里是 null 断口，不补 0', async () => {
    state.snaps = [
      {
        online: true,
        metrics: JSON.stringify({ online: true, cpu_usage: 40, mem_usage: 50 }),
        snap_at: '2026-10-12T10:00:00Z',
      },
      { online: false, metrics: JSON.stringify({ online: false }), snap_at: '2026-10-12T10:01:00Z' },
      {
        online: true,
        metrics: JSON.stringify({ online: true, cpu_usage: 60, mem_usage: 70 }),
        snap_at: '2026-10-12T10:02:00Z',
      },
    ]
    const st = useDeviceDetail(ref(true), ref({ id: 1, type: 'host', name: 'h', last_online: false } as never))
    await st.load()
    expect(st.cpuSeries.value).toEqual([40, null, 60])
    expect(st.memSeries.value).toEqual([50, null, 70])
  })

  it('正控：全部在线时序列无 null', async () => {
    state.snaps = [
      { online: true, metrics: JSON.stringify({ online: true, cpu_usage: 40 }), snap_at: '2026-10-12T10:00:00Z' },
      { online: true, metrics: JSON.stringify({ online: true, cpu_usage: 60 }), snap_at: '2026-10-12T10:01:00Z' },
    ]
    const st = useDeviceDetail(ref(true), ref({ id: 1, type: 'host', name: 'h', last_online: true } as never))
    await st.load()
    expect(st.cpuSeries.value).toEqual([40, 60])
  })
})

// 接线层：后端把 total/truncated 交回来了，composable 若不往下传，
// 渲染层那段提示就永远不会出现（组件测试全绿也掩盖不了）。
describe('useDeviceDetail：趋势窗口账面接到响应', () => {
  function threeSnaps() {
    return [1, 2, 3].map((i) => ({
      online: true,
      metrics: JSON.stringify({ online: true, cpu_usage: i * 10 }),
      snap_at: `2026-10-12T10:0${i}:00Z`,
    }))
  }

  function detail() {
    return useDeviceDetail(ref(true), ref({ id: 1, type: 'host', name: 'h', last_online: true } as never))
  }

  it('响应报截断：shown/total/truncated 一并交给界面', async () => {
    state.snaps = threeSnaps()
    state.total = 860
    state.truncated = true
    const st = detail()
    await st.load()
    expect(st.trendWindow.value).toEqual({ shown: 3, total: 860, truncated: true })
  })

  it('总数读不出：truncated 仍要传下去，total 不补 0', async () => {
    state.snaps = threeSnaps()
    state.total = undefined
    state.truncated = true
    const st = detail()
    await st.load()
    expect(st.trendWindow.value.shown).toBe(3)
    expect(st.trendWindow.value.truncated).toBe(true)
    expect(st.trendWindow.value.total).toBeUndefined()
  })

  it('正控：一次拿全时 truncated 为假', async () => {
    state.snaps = threeSnaps()
    state.total = 3
    state.truncated = false
    const st = detail()
    await st.load()
    expect(st.trendWindow.value.truncated).toBe(false)
    expect(st.trendWindow.value.total).toBe(3)
  })
})

describe('DeviceCard：无实指标快照走 no-data 分支', () => {
  const offlineTarget = {
    id: 1,
    name: 'NAS-01',
    type: 'nas',
    host: '',
    last_online: false,
    last_checked_at: '2026-10-12T10:00:00Z',
    last_error: 'dial tcp 192.168.1.5:5000: connect: connection refused',
    vendor: '',
    protocol: '',
    port: 0,
  } as unknown as MonitorTarget

  const onlineTarget = { ...offlineTarget, last_online: true, last_error: '' } as MonitorTarget

  it('离线快照 {online:false}：显示 last_error，不出现 0.0%', () => {
    const el = mountNode(DeviceCard, { target: offlineTarget, metrics: { online: false } as Metrics })
    const t = text(el)
    expect(t).toContain('connection refused')
    expect(t).not.toContain('0.0%')
  })

  it('仅 online 的探活快照：显示空数据提示，不出现 0.0%', () => {
    // 探活成功会清空 last_error（RecordSnapshot），此时卡片应显示空数据提示。
    const probedTarget = { ...offlineTarget, last_online: true, last_error: '' } as MonitorTarget
    const el = mountNode(DeviceCard, { target: probedTarget, metrics: { online: true } as Metrics })
    const t = text(el)
    expect(t).toContain('点击探活')
    expect(t).not.toContain('0.0%')
  })

  it('正控：带指标快照正常渲染 45.0%，不显示 last_error', () => {
    const el = mountNode(DeviceCard, {
      target: onlineTarget,
      metrics: { online: true, cpu_usage: 45, mem_usage: 60, mem_total: 1073741824, mem_used: 536870912 } as Metrics,
    })
    const t = text(el)
    expect(t).toContain('45.0%')
    expect(t).toContain('60.0%')
    expect(t).not.toContain('connection refused')
  })
})

// 趋势图取数账面（渲染层）：后端一次只给最近 limit 条，界面若不区分「窗口」与「全部历史」，
// 用户就会把「最近 120 次采集」读成「这台设备从建号到现在的曲线」。
// 计数读不出时不能显示 `共 undefined 次`，也不能干脆不说。
describe('DeviceHostDetail：趋势窗口被截断要在图上说清', () => {
  const baseProps = {
    target: { id: 1, name: 'NAS-01', type: 'host', host: '127.0.0.1', last_online: true } as unknown as MonitorTarget,
    latest: { online: true, cpu_usage: 40, mem_usage: 50 } as Metrics,
    deviceId: '1',
    collectTime: '2026-10-12 10:00:00',
    cpuSeries: [30, 40, 50],
    memSeries: [45, 50, 55],
    diskUsage: 60,
    diskTotal: 100,
    diskUsed: 60,
    hottestTemp: undefined,
    tempEmptyHint: '暂无温度数据',
    resolvedProtocol: 'local',
    canPower: false,
    powerLoading: '',
  }

  function mountHost(trendWindow: Record<string, unknown> | undefined): HTMLElement {
    return mountNode(DeviceHostDetail, trendWindow ? { ...baseProps, trendWindow } : baseProps)
  }

  it('被上限截断：说出「已画多少 / 共多少」', () => {
    const el = mountHost({ shown: 120, total: 860, truncated: true })
    const note = el.querySelector('.trend-note')
    expect(note).not.toBeNull()
    expect(note?.textContent ?? '').toContain('120')
    expect(note?.textContent ?? '').toContain('860')
    el.remove()
  })

  it('总数读不出：说「可能还有更多」，不渲染 undefined', () => {
    const el = mountHost({ shown: 120, truncated: true })
    const note = el.querySelector('.trend-note')
    expect(note).not.toBeNull()
    expect(note?.textContent ?? '').toContain('读取失败')
    expect(text(el)).not.toContain('undefined')
    el.remove()
  })

  it('正控：一次拿全时不显示窗口提示', () => {
    const el = mountHost({ shown: 12, total: 12, truncated: false })
    expect(el.querySelector('.trend-note')).toBeNull()
    el.remove()
  })
})
