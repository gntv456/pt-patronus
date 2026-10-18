import { describe, it, expect, vi, beforeEach } from 'vitest'

// 字幕 api 的接线回归：请求参数由这里唯一决定，界面拿不到超时/归一后的形状就无从排查。
const { getMock, postMock } = vi.hoisted(() => ({ getMock: vi.fn(), postMock: vi.fn() }))

vi.mock('./http', () => ({ http: { get: getMock, post: postMock } }))

import { subtitleApi } from './subtitle'

// 后端链层给字幕源的预算（internal/chain/subtitle.go）：下载 30s。
// 客户端超时若低于服务端预算，「站点慢但会成功」会被掐成「请求超时，请稍后重试」，
// 而后端仍在跑完 —— 用户以为失败、文件也不落地（同 #111 首屏超时的量法）。
const BACKEND_DOWNLOAD_BUDGET = 30000

describe('subtitleApi 超时与归一', () => {
  beforeEach(() => {
    getMock.mockReset()
    postMock.mockReset()
  })

  it('download 的请求超时不得短于后端 30s 预算', async () => {
    postMock.mockResolvedValue({
      data: new Blob(['1\n00:00:01 --> 00:00:02\nhi\n']),
      headers: { 'content-disposition': 'attachment; filename="ch.srt"' },
    })
    const { filename } = await subtitleApi.download({ source: 'subhd', source_id: '1' })
    expect(filename).toBe('ch.srt')
    const opts = postMock.mock.calls[0][2] as { responseType: string; timeout?: number }
    expect(opts.responseType).toBe('blob')
    // 不写 timeout 时读到的是 undefined —— 这条断言正是钉住「显式覆盖全局 15s」这件事。
    expect(opts.timeout).toBeGreaterThanOrEqual(BACKEND_DOWNLOAD_BUDGET)
  })

  it('search 带 q/language，且后端没给 results 时归一成空数组（不是 undefined）', async () => {
    getMock.mockResolvedValue({ data: {} })
    const res = await subtitleApi.search('某片', 'chs')
    expect(res).toEqual([])
    expect(getMock.mock.calls[0][1]).toEqual({ params: { q: '某片', language: 'chs' } })
  })
})
