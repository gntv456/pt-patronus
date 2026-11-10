// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { describe, it, expect } from 'vitest'
import { applyDiff, type TorrentSnapshot, type TorrentDiff } from '@/composables/useWs'
import type { Torrent } from '@/api/downloader'

function makeTorrent(hash: string, name = hash): Torrent {
  return {
    hash,
    name,
    size: 0,
    progress: 0,
    ratio: 0,
    state: 'completed',
    dl_rate: 0,
    up_rate: 0,
    save_path: '',
    category: '',
    tags: [],
    num_seeds: 0,
    num_leechs: 0,
    num_complete: 0,
    num_incomplete: 0,
    availability: 0,
    ratio_limit: -1,
    seeding_time_limit: -1,
    force_start: false,
    auto_tmm: false,
    sequential: false,
    first_last_piece: false,
    priority: 0,
    added_at: '',
    completed_at: '',
  }
}

function makeSnap(id: number, torrents: Torrent[] = []): TorrentSnapshot {
  return { downloader_id: id, name: 'dl' + id, error: '', torrents }
}

describe('applyDiff', () => {
  it('新增整个下载器', () => {
    const r = applyDiff([], { added: [makeSnap(1, [makeTorrent('h1')])] })
    expect(r).toHaveLength(1)
    expect(r[0].downloader_id).toBe(1)
    expect(r[0].torrents).toHaveLength(1)
  })

  it('删除下载器', () => {
    const r = applyDiff([makeSnap(1), makeSnap(2)], { removed: [1] })
    expect(r).toHaveLength(1)
    expect(r[0].downloader_id).toBe(2)
  })

  it('同轮种子增/删/改并存', () => {
    const cur = [makeSnap(1, [makeTorrent('h1', 'old'), makeTorrent('h2')])]
    const diff: TorrentDiff = {
      changed: [
        {
          downloader_id: 1,
          error: '',
          added: [makeTorrent('h3')],
          changed: [makeTorrent('h1', 'new')],
          removed: ['h2'],
        },
      ],
    }
    const r = applyDiff(cur, diff)
    const ts = r[0].torrents
    expect(ts.map((t) => t.hash).sort()).toEqual(['h1', 'h3'])
    expect(ts.find((t) => t.hash === 'h1')?.name).toBe('new')
  })

  it('changed 的 error/stats 覆盖式更新', () => {
    const cur = [makeSnap(1, [makeTorrent('h1')])]
    cur[0].error = 'old err'
    const r = applyDiff(cur, { changed: [{ downloader_id: 1, error: '', stats: { dl_rate: 9, up_rate: 8 } }] })
    expect(r[0].error).toBe('')
    expect(r[0].stats).toEqual({ dl_rate: 9, up_rate: 8 })
  })

  it('空 diff 返回等价新数组（不 mutate 原数组）', () => {
    const cur = [makeSnap(1, [makeTorrent('h1')])]
    const r = applyDiff(cur, {})
    expect(r).toHaveLength(1)
    expect(r[0].torrents).toHaveLength(1)
    expect(r).not.toBe(cur)
  })

  it('出错下载器 snapshot.torrents 为 null（Go nil slice → JSON null）不崩溃，归一为 []', () => {
    // 后端 aggregate：下载器连接失败时 snap.Torrents 未赋值（nil），JSON 序列化为 "torrents": null。
    const nullSnap = {
      downloader_id: 2,
      name: 'err-dl',
      error: 'connection refused',
      torrents: null as unknown as Torrent[],
    }
    // added 路径：[...null] 会抛 "not iterable"，须归一
    const r1 = applyDiff([], { added: [nullSnap] })
    expect(r1).toHaveLength(1)
    expect(r1[0].torrents).toEqual([])
    // cur 路径： torrents_full 已在 onMessage 归一，applyDiff 接收到的 cur 不应含 null torrents；
    // 但 changed 增删改若 prev.torrents 异常为 null 也需兜底——此处验证 prev 已归一后正常工作。
    const normalizedCur = [{ ...nullSnap, torrents: [] as Torrent[] }]
    const r2 = applyDiff(normalizedCur, {
      changed: [{ downloader_id: 2, error: '', added: [], changed: [], removed: [] }],
    })
    expect(r2[0].torrents).toEqual([])
    expect(r2[0].error).toBe('')
  })
})
