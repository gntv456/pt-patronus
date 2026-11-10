// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { beforeEach, describe, expect, it, vi } from 'vitest'

// http 层整体 mock 掉：这里只验 backupApi 往外发什么参数，不打网络。
const { getMock, postMock } = vi.hoisted(() => ({ getMock: vi.fn(), postMock: vi.fn() }))

vi.mock('./http', () => ({ http: { get: getMock, post: postMock } }))

import { FAILURE_SHOWN_LIMIT, backupApi, importFailureNotice, importOutcomeKind } from './backup'
import zhCN from '../i18n/locales/zh-CN'
import enUS from '../i18n/locales/en-US'

// 备份导入的失败清单：页面只列前几条，但「一共几条」必须一起说。
// MoviePilot / .ptd 两条外部格式路径：0 条不是「导入完成」。
describe('importOutcomeKind 零条目不算完成', () => {
  it('站点与下载器都为 0（或后端没给）=> none', () => {
    expect(importOutcomeKind(0)).toBe('none')
    expect(importOutcomeKind(0, 0)).toBe('none')
    expect(importOutcomeKind(undefined)).toBe('none')
  })
  it('任一计数为正 => done', () => {
    expect(importOutcomeKind(3)).toBe('done')
    expect(importOutcomeKind(0, 2)).toBe('done')
  })
  it('两语都有 backup.importNothing，且英文不是照抄中文', () => {
    const z = (zhCN as { backup?: { importNothing?: string } }).backup?.importNothing ?? ''
    const e = (enUS as { backup?: { importNothing?: string } }).backup?.importNothing ?? ''
    expect(z).not.toBe('')
    expect(e).not.toBe('')
    expect(e).not.toBe(z)
  })
})

describe('importFailureNotice 失败清单账面', () => {
  it('超过展示上限时，总数仍是全部而不是列出的那几条', () => {
    const many = Array.from({ length: 12 }, (_, i) => `站点 site-${i}：Cookie 字段缺失`)
    const plan = importFailureNotice(many)
    expect(plan.total).toBe(12)
    expect(plan.shown).toHaveLength(FAILURE_SHOWN_LIMIT)
    expect(plan.shown[0]).toContain('site-0')
  })

  it('没超过上限时全部列出，总数等于条数', () => {
    const few = ['下载器 qB：地址非法', '站点 x：已存在']
    expect(importFailureNotice(few)).toEqual({ total: 2, shown: few })
  })

  it('后端没给 failures / 给的不是数组时归零，不能崩也不能谎报有失败', () => {
    expect(importFailureNotice(undefined)).toEqual({ total: 0, shown: [] })
    expect(importFailureNotice({ a: 1 })).toEqual({ total: 0, shown: [] })
  })
})

// 占位符一致性：localesConsistency 只比 key 数量，不比句子内的占位符集合。
// 少一个 {total} 时英文用户会直接看到字面量 "{total}"（vue-i18n 不报错）。
describe('backup.importFailures 文案占位符', () => {
  const keys = (s: string) =>
    [...s.matchAll(/\{(\w+)\}/g)]
      .map((m) => m[1])
      .sort()
      .join(',')

  const zh = (zhCN as { backup?: { importFailures?: string } }).backup?.importFailures ?? ''
  const en = (enUS as { backup?: { importFailures?: string } }).backup?.importFailures ?? ''

  it('两语都用同一组占位符，且总数与展示数都在句子里', () => {
    expect(zh, 'zh-CN 缺 backup.importFailures').not.toBe('')
    expect(keys(en), keys(zh)).toBe(keys(zh))
    for (const k of ['total', 'shown', 'list']) {
      expect(zh).toContain(`{${k}}`)
      expect(en).toContain(`{${k}}`)
    }
  })

  it('文案不许把「列出的几条」说成全部（判据：必须出现总数槽位）', () => {
    expect(zh).toMatch(/\{total\}/)
    expect(en).toMatch(/\{total\}/)
  })
})

// #239 三条导入必须显式关掉客户端超时：服务端不读请求 ctx（取消不了导入），
// 全局 15s 只会把「其实已经导完」显示成失败，用户再点一次就是重复导入。
describe('backupApi 三条导入的超时', () => {
  beforeEach(() => {
    postMock.mockReset()
    postMock.mockResolvedValue({ data: { imported_sites: 2 } })
  })

  it('importOwn 关掉超时，并且口令仍然走请求头不进 URL', async () => {
    await backupApi.importOwn({ v: 1 }, 'pw-1')
    const opts = postMock.mock.calls[0][2] as { timeout: number; headers: Record<string, string> }
    expect(opts.timeout).toBe(0)
    expect(opts.headers['X-Backup-Password']).toBe('pw-1')
  })

  it('不填口令时也必须带 timeout（不能退回 undefined 吃全局 15s）', async () => {
    await backupApi.importOwn({ v: 1 })
    const opts = postMock.mock.calls[0][2] as { timeout?: number }
    expect(opts.timeout).toBe(0)
  })

  it('MoviePilot 与 .ptd 同样关掉超时', async () => {
    await backupApi.importMoviePilot([])
    expect((postMock.mock.calls[0][2] as { timeout: number }).timeout).toBe(0)
    await backupApi.importPTD(new File(['x'], 'a.ptd'))
    expect((postMock.mock.calls[1][2] as { timeout: number }).timeout).toBe(0)
  })
})
