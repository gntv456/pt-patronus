import { describe, it, expect, vi, beforeEach } from 'vitest'

// 用 vi.hoisted 让 mock 句柄在 vi.mock 工厂（被提升到文件顶部）执行时已就绪。
const { getMock } = vi.hoisted(() => ({ getMock: vi.fn() }))
vi.mock('./http', () => ({ http: { get: getMock } }))

import { recommendApi, recommendSources, recommendCategories } from './recommend'

// 推荐源/分类静态数据 + feed() 的请求参数拼装。既守住数据形状，也间接验证
// TMDB「动漫」分类在前端确实被列出（配合后端 discover/tv with_genres=16）。
describe('recommend 静态数据', () => {
  it('源含「全部」+豆瓣/TMDB/Bangumi/寸光集', () => {
    expect(recommendSources.map((s) => s.key)).toEqual(['', 'douban', 'tmdb', 'bangumi', 'cunguangji'])
  })

  it('分类含动漫（anime）与短剧（duanju）', () => {
    expect(recommendCategories.some((c) => c.key === 'anime')).toBe(true)
    expect(recommendCategories.some((c) => c.key === 'duanju')).toBe(true)
  })
})

describe('recommendApi.feed', () => {
  beforeEach(() => getMock.mockReset())

  it('透传 source/category/page 参数并展开 results', async () => {
    getMock.mockResolvedValue({ data: { results: [{ source: 'tmdb', source_id: '1' }] } })

    const out = await recommendApi.feed('tmdb', 'anime', 2, true)

    expect(getMock).toHaveBeenCalledTimes(1)
    const [url, cfg] = getMock.mock.calls[0]
    expect(url).toBe('/media/recommend')
    expect(cfg.params).toEqual({ source: 'tmdb', category: 'anime', page: 2, refresh: 1 })
    expect(out).toHaveLength(1)
    expect(out[0].source).toBe('tmdb')
  })

  it('非刷新时不带 refresh 参数', async () => {
    getMock.mockResolvedValue({ data: { results: [] } })

    await recommendApi.feed('', 'movie')

    const cfg = getMock.mock.calls[0][1]
    expect(cfg.params).toEqual({ source: '', category: 'movie', page: 1 })
    expect(cfg.params).not.toHaveProperty('refresh')
  })
})
