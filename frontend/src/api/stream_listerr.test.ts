// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// @vitest-environment jsdom
// 流式 done 帧的 list_error 必须变成一次 reject —— 视图里已有的 catch 才会弹原文。
// 这里不打桩 streamSSE（那等于把要验的分帧/解析换掉），而是喂真 SSE 字节给真的 streamSSE。
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { haremApi } from './harem'
import { medalWallApi } from './medal-wall'

const REASON = '读取站点清单失败: no such table: sites'

// 用真的 fetch 形状造一段 SSE 响应体（ok/body/reader），让 sse.ts 自己分帧。
function sseResponse(frames: string[]): Response {
  const enc = new TextEncoder()
  const body = new ReadableStream<Uint8Array>({
    start(controller) {
      for (const f of frames) controller.enqueue(enc.encode(f))
      controller.close()
    },
  })
  return { ok: true, status: 200, body, text: async () => '' } as unknown as Response
}

let frames: string[] = []
const fetchMock = vi.fn(async () => sseResponse(frames))

beforeEach(() => {
  frames = []
  fetchMock.mockClear()
  vi.stubGlobal('fetch', fetchMock)
  localStorage.setItem('token', 't')
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('后宫 / 勋章墙流式：done 帧的 list_error 不许被丢掉', () => {
  it('harem：done 带 list_error ⇒ reject 给视图 catch，并且不调 onOverview', async () => {
    frames = [
      'event: batch\ndata: {"results":[{"site_id":1,"site_name":"A","harem_count":3}]}\n\n',
      `event: done\ndata: {"overview":{"total_sites":1},"list_error":"${REASON}"}\n\n`,
    ]
    const overview = vi.fn()
    const items = vi.fn()
    await expect(haremApi.allStream(false, items, overview)).rejects.toThrow(REASON)
    expect(overview).not.toHaveBeenCalled()
    expect(items).toHaveBeenCalledTimes(1) // 已经渐进显示的那站卡片不该被回滚
  })

  it('harem 正控：干净 done 帧 ⇒ 汇总照常交出，不报故障', async () => {
    frames = [
      'event: batch\ndata: {"results":[{"site_id":1,"site_name":"A","harem_count":3}]}\n\n',
      'event: done\ndata: {"overview":{"total_sites":1,"total_invitees":3}}\n\n',
    ]
    const overview = vi.fn()
    await haremApi.allStream(false, vi.fn(), overview)
    expect(overview).toHaveBeenCalledWith(expect.objectContaining({ total_sites: 1 }))
  })

  it('medalwall：done 带 list_error ⇒ reject（这个客户端从前整帧都不看）', async () => {
    frames = [
      'event: batch\ndata: {"results":[{"site_id":1,"site_name":"A","medal_id":"m1","name":"勋章","price":10}]}\n\n',
      `event: done\ndata: {"list_error":"${REASON}"}\n\n`,
    ]
    const cards = vi.fn()
    await expect(medalWallApi.allStream(false, cards)).rejects.toThrow(REASON)
    expect(cards).toHaveBeenCalledTimes(1)
  })

  it('medalwall 正控：没有 list_error 的 done 帧 ⇒ 正常收尾，不误报', async () => {
    frames = [
      'event: batch\ndata: {"results":[{"site_id":1,"site_name":"A","medal_id":"m1","name":"勋章","price":10}]}\n\n',
      'event: done\ndata: {}\n\n',
    ]
    const cards = vi.fn()
    await medalWallApi.allStream(false, cards)
    expect(cards).toHaveBeenCalledTimes(1)
  })
})
