import { beforeEach, describe, expect, it, vi } from 'vitest'

// 只验 downloaderApi 怎么读 /torrents 的响应体：http 层整体 mock，不打网络。
const { getMock } = vi.hoisted(() => ({ getMock: vi.fn() }))

vi.mock('./http', () => ({ http: { get: getMock } }))

import { downloaderApi } from './downloader'

// 后端把「读不到下载器」压成 200 + {torrents: [], error: '中文原因'}（downloader.go:154），
// 状态码是 200、数组是空——两个都不说话，只有 error 这个字段说话。
// 这条用例钉的是：api 层必须把 error 变成异常抛出去，让调用方（整理页 loadTorrents 的
// ElMessage.error(e.message)）能把中文原因弹出来，而不是画「0 个任务」。
describe('downloaderApi.torrents 不把读失败当成「没有种子」', () => {
  beforeEach(() => getMock.mockReset())

  it('200 + error ⇒ 抛出后端原文，且真的打了这个端点', async () => {
    getMock.mockResolvedValue({
      data: { torrents: [], total: 0, error: 'qBittorrent 连接被拒绝，请确认下载器已启动' },
    })
    await expect(downloaderApi.torrents(7)).rejects.toThrow('qBittorrent 连接被拒绝，请确认下载器已启动')
    expect(getMock).toHaveBeenCalledWith('/downloaders/7/torrents')
  })

  it('正常载荷 ⇒ 原样返回数组（不误伤）', async () => {
    getMock.mockResolvedValue({ data: { torrents: [{ hash: 'a1', name: 'movie' }], total: 1 } })
    await expect(downloaderApi.torrents(1)).resolves.toEqual([{ hash: 'a1', name: 'movie' }])
  })

  it('error 是空串/缺省 ⇒ 不算失败；后端没给 torrents 键 ⇒ 空数组', async () => {
    getMock.mockResolvedValue({ data: { torrents: [], error: '' } })
    await expect(downloaderApi.torrents(2)).resolves.toEqual([])
    getMock.mockResolvedValue({ data: {} })
    await expect(downloaderApi.torrents(3)).resolves.toEqual([])
  })
})
