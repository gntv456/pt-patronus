// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { beforeEach, describe, expect, it, vi } from 'vitest'

// 本机 Docker 资源条的「停更仍画旧值」用例：refreshStats 是 5 秒轮询，
// 以前失败全静默（`catch {}`），CPU/内存就停在最后一次成功读数上，和实时值一模一样。
// 这里验的是修完之后：当前读数被清掉（界面回落到既有的「—」），历史曲线不动。
const { localStatsMock, localListMock, statsMock, localStatusMock, localProjectsMock } = vi.hoisted(() => ({
  localStatsMock: vi.fn(),
  localListMock: vi.fn(),
  statsMock: vi.fn(),
  localStatusMock: vi.fn(),
  // refresh() 的 finally 里会顺手取 compose 项目数；不给它会以 TypeError 失败（红得没道理）。
  localProjectsMock: vi.fn(),
}))

vi.mock('@/api/docker', () => ({
  dockerApi: {
    localStats: localStatsMock,
    localList: localListMock,
    stats: statsMock,
    localStatus: localStatusMock,
    localProjects: localProjectsMock,
  },
}))

import { useDockerData } from './useDockerData'

describe('本机统计轮询失败不再把旧读数当实时值', () => {
  beforeEach(() => {
    for (const m of [localStatsMock, localListMock, statsMock, localStatusMock, localProjectsMock]) m.mockReset()
    localProjectsMock.mockResolvedValue({ projects: [] })
  })

  it('成功一轮有读数，下一轮失败 ⇒ 当前读数清空、历史保留', async () => {
    const d = useDockerData(() => 'local')
    localStatsMock.mockResolvedValueOnce({
      stats: [{ Name: 'qbittorrent', CPUPerc: '12.00%', MemPerc: '3.00%', MemUsage: '12 MiB' }],
    })
    await d.refreshStats()
    expect(d.stats.value).toHaveLength(1)
    expect(d.cpuHistoryFor('qbittorrent')).toEqual([12])

    localStatsMock.mockRejectedValueOnce(new Error('连不上本机 Docker 引擎'))
    await d.refreshStats()
    expect(d.stats.value).toEqual([])
    // 历史是「采到过什么」的账，不该被一次失败抹掉；丢的是「现在是多少」。
    expect(d.cpuHistoryFor('qbittorrent')).toEqual([12])
    expect(d.statFor('qbittorrent')).toBeUndefined()
  })

  it('恢复成功后重新有读数（清空不是终点）', async () => {
    const d = useDockerData(() => 'local')
    localStatsMock.mockRejectedValueOnce(new Error('引擎不在'))
    await d.refreshStats()
    expect(d.stats.value).toEqual([])

    localStatsMock.mockResolvedValueOnce({
      stats: [{ Name: 'nginx', CPUPerc: '1.50%', MemPerc: '0.50%', MemUsage: '4 MiB' }],
    })
    await d.refreshStats()
    expect(d.statFor('nginx')?.CPUPerc).toBe('1.50%')
  })

  // #245：停更要说得出「为什么」与「上次成功是几点」，原因用后端给的中文，界面不自己编。
  it('停更账面：轮询失败留下后端原文，成功一次就复位', async () => {
    const d = useDockerData(() => 'local')
    expect(d.statsError.value).toBe('')
    expect(d.statsLastOkAt.value).toBe(0) // 从没成功过 ⇒ 不许显示「上次成功时间」

    localStatsMock.mockRejectedValueOnce({
      response: { data: { error: '连不上本机 Docker 引擎：默认走 /var/run/docker.sock' } },
    })
    await d.refreshStats()
    expect(d.statsError.value).toContain('连不上本机 Docker 引擎')
    expect(d.statsLastOkAt.value).toBe(0)

    const t0 = Date.now()
    localStatsMock.mockResolvedValueOnce({ stats: [] })
    await d.refreshStats()
    expect(d.statsError.value).toBe('')
    expect(d.statsLastOkAt.value).toBeGreaterThanOrEqual(t0)
  })

  // 本机探测失败时后端给的是具体原因+出路（#240），从前这里只留 available=false，
  // 那句「可设 DOCKER_HOST」在 Web 上没有任何读者。
  it('local/status 的 error 被留成可读的提示，可用时清空', async () => {
    const d = useDockerData(() => 'local')
    localStatusMock.mockResolvedValueOnce({
      available: false,
      endpoint: '/var/run/docker.sock',
      error:
        'socket 文件不存在：/var/run/docker.sock（Windows/macOS 原生运行本程序时该地址不存在，可设 DOCKER_HOST=tcp://127.0.0.1:2375 后重启）',
    })
    await d.initLocalAvailability()
    expect(d.localAvailable.value).toBe(false)
    expect(d.localStatusHint.value).toContain('DOCKER_HOST')

    localStatusMock.mockResolvedValueOnce({ available: true, endpoint: '/var/run/docker.sock' })
    await d.initLocalAvailability()
    expect(d.localStatusHint.value).toBe('')
  })

  it('探测请求本身失败时退回异常原文，不空着', async () => {
    const d = useDockerData(() => 'local')
    localStatusMock.mockRejectedValueOnce(new Error('登录态已更新，请重新登录'))
    await d.initLocalAvailability()
    expect(d.localAvailable.value).toBe(false)
    expect(d.localStatusHint.value).toBe('登录态已更新，请重新登录')
  })

  // 整页刷新是第二个出口（统计失败不该拖走列表，但要留痕）：两处共用同一份判定。
  it('refresh() 里统计失败同样留痕，容器列表不受影响', async () => {
    const d = useDockerData(() => 'local')
    localListMock.mockResolvedValueOnce({
      containers: [{ ID: 'abcdef123456', Names: 'qb' }],
      images: [],
      volumes: [],
      networks: [],
    })
    localStatsMock.mockRejectedValueOnce({ response: { data: { error: '引擎答了但这次失败' } } })
    await d.refresh()
    expect(d.data.value.containers).toHaveLength(1)
    expect(d.stats.value).toEqual([])
    expect(d.statsError.value).toContain('引擎答了但这次失败')
  })
})
