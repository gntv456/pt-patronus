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
})
