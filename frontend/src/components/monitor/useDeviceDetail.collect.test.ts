import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'

// 这一条补 #251 的接线证明：api 层抛出 collect 失败，不代表设备详情真的把那句原因显示给人——
// 从前调用点是 `.catch(() => null)`，返回值与异常一起丢掉，界面照旧画历史快照当最新。
// 这里不起组件、不碰网络：只替换 monitorApi 的 collect/metrics 两个方法
// （parseMetrics 等走真实实现，用 importOriginal 保住），element-plus 整块 mock。
const { collectMock, metricsMock, msg } = vi.hoisted(() => ({
  collectMock: vi.fn(),
  metricsMock: vi.fn(),
  msg: { warning: vi.fn(), error: vi.fn(), success: vi.fn() },
}))

vi.mock('@/api/monitor', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api/monitor')>()
  return { ...actual, monitorApi: { ...actual.monitorApi, collect: collectMock, metrics: metricsMock } }
})
vi.mock('element-plus', () => ({ ElMessage: msg, ElMessageBox: { confirm: vi.fn() } }))

import { useDeviceDetail } from './useDeviceDetail'

const target = { id: 7, name: 'NAS', type: 'nas' } as never

describe('设备详情把「这次没采到」与「刚采到新数」分开', () => {
  beforeEach(() => {
    collectMock.mockReset()
    metricsMock.mockReset()
    msg.warning.mockReset()
    metricsMock.mockResolvedValue({ snapshots: [], total: 0, limit: 120, truncated: false })
  })

  it('采集失败 ⇒ 弹含后端原因与「历史数据」的提示，历史仍然去拉', async () => {
    collectMock.mockRejectedValue(new Error('SSH 连不上：connection refused'))
    const d = useDeviceDetail(ref(true), ref(target))
    await d.load()
    expect(msg.warning).toHaveBeenCalledTimes(1)
    const text = String(msg.warning.mock.calls[0][0])
    expect(text).toContain('SSH 连不上')
    expect(text).toContain('历史数据')
    // 失败不该把详情整页拖没——历史照旧要能看，只是要标清它是历史。
    expect(metricsMock).toHaveBeenCalledTimes(1)
  })

  it('采集成功 ⇒ 一个字都不弹（正控：闸门不许把每次打开都变成告警）', async () => {
    collectMock.mockResolvedValue({ ok: true, metrics: { cpu_usage: 3 } })
    const d = useDeviceDetail(ref(true), ref(target))
    await d.load()
    expect(msg.warning).not.toHaveBeenCalled()
    expect(metricsMock).toHaveBeenCalledTimes(1)
  })
})
