import { beforeEach, describe, expect, it, vi } from 'vitest'

const { postMock } = vi.hoisted(() => ({ postMock: vi.fn() }))

vi.mock('./http', () => ({ http: { post: postMock } }))

import { monitorApi } from './monitor'

// 后端 monitor.go:113 在采集失败时回的是 200 + {ok:false, error:'中文原因'}：
// 状态码不说、metrics 字段缺席也不算异常。api 层以前直接把整个信封交下去，
// 设备详情因此把「这次没采到」画成「刚采到新数」（#251）。
describe('monitorApi.collect 不把采集失败交成一次成功', () => {
  beforeEach(() => postMock.mockReset())

  it('200 + ok:false ⇒ 抛出后端原文，并且真的 POST 打了这个端点', async () => {
    postMock.mockResolvedValue({ data: { ok: false, error: 'SSH 连不上：credential 已删除' } })
    await expect(monitorApi.collect(7)).rejects.toThrow('SSH 连不上：credential 已删除')
    expect(postMock).toHaveBeenCalledWith('/monitors/7/collect')
  })

  it('ok:false 但后端没给原因 ⇒ 退回「采集失败」，不许静默 resolve', async () => {
    postMock.mockResolvedValue({ data: { ok: false } })
    await expect(monitorApi.collect(8)).rejects.toThrow('采集失败')
  })

  it('ok:true ⇒ 原样带回信封（metrics 照旧可读），不误伤', async () => {
    postMock.mockResolvedValue({ data: { ok: true, metrics: { cpu_usage: 12 } } })
    await expect(monitorApi.collect(9)).resolves.toEqual({ ok: true, metrics: { cpu_usage: 12 } })
  })
})
