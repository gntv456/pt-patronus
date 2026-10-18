import { describe, expect, it, vi } from 'vitest'

const { postMock } = vi.hoisted(() => ({ postMock: vi.fn() }))
vi.mock('./http', () => ({ http: { post: postMock } }))

import { siteApi } from './site'

describe('siteApi check-in timeouts', () => {
  it('allows a single site to use the backend 30s site budget', async () => {
    postMock.mockResolvedValueOnce({ data: { site_id: 7, success: true, bonus: 0 } })

    await siteApi.checkin(7)

    expect(postMock).toHaveBeenCalledWith('/sites/7/checkin', undefined, { timeout: 45_000 })
  })

  it('allows the complete concurrent batch to finish', async () => {
    postMock.mockResolvedValueOnce({ data: { results: [] } })

    await siteApi.checkinAll()

    expect(postMock).toHaveBeenCalledWith('/sites/checkin', undefined, { timeout: 300_000 })
  })

  // #258：后端读不出站清单时回 {results: [], list_error:'原因'}。
  // api 层以前只取 results ⇒ 「读不到」在下一层被讲成「没有启用的站点」。
  it('keeps the read-failure reason instead of collapsing it into an empty list', async () => {
    postMock.mockResolvedValueOnce({
      data: { results: [], list_error: '读取启用站点清单失败: no such table: sites' },
    })
    await expect(siteApi.checkinAll()).resolves.toEqual({
      results: [],
      listError: '读取启用站点清单失败: no such table: sites',
    })
  })

  it('null results (老载荷) 仍归一成空数组，缺 list_error 时为空串', async () => {
    postMock.mockResolvedValueOnce({ data: { results: null } })
    await expect(siteApi.checkinAll()).resolves.toEqual({ results: [], listError: '' })
  })

  it('正常逐站结果原样带出（不被新信封丢掉）', async () => {
    postMock.mockResolvedValueOnce({ data: { results: [{ site_id: 1, site_name: 'A', success: true }] } })
    await expect(siteApi.checkinAll()).resolves.toEqual({
      results: [{ site_id: 1, site_name: 'A', success: true }],
      listError: '',
    })
  })
})
