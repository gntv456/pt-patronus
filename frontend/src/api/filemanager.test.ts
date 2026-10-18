import { beforeEach, describe, expect, it, vi } from 'vitest'

const { getMock } = vi.hoisted(() => ({ getMock: vi.fn() }))

vi.mock('./http', () => ({ http: { get: getMock } }))

import { fileApi, targetReadFailedNotice } from './filemanager'

// 后端 filemanager.go:105 在「读不到目录」时回 200 + {entries: [], error: '中文原因'}。
// api 层以前只取 entries ⇒ 下游把「读不到」当成「目录是空的」：
// 主列表少一行横幅还是小事，useFileOps.doMoveCopy 的同名覆盖预检会因此直接放行，
// 而后端 move/copy 是同名静默覆盖（不可逆）。
describe('fileApi.files 不把读失败当成「目录为空」', () => {
  beforeEach(() => getMock.mockReset())

  it('200 + error ⇒ 抛出后端原文，并且真的带 path 打了这个端点', async () => {
    getMock.mockResolvedValue({ data: { entries: [], error: 'WebDAV 目录列举失败：403 Forbidden' } })
    await expect(fileApi.files(4, '/media')).rejects.toThrow('WebDAV 目录列举失败：403 Forbidden')
    expect(getMock).toHaveBeenCalledWith('/storages/4/files', { params: { path: '/media' } })
  })

  it('正常载荷 ⇒ 原样返回数组；后端没给 entries 键 ⇒ 空数组（不误伤）', async () => {
    getMock.mockResolvedValue({ data: { entries: [{ name: 'a.mkv', is_dir: false }] } })
    await expect(fileApi.files(1)).resolves.toEqual([{ name: 'a.mkv', is_dir: false }])
    getMock.mockResolvedValue({ data: {} })
    await expect(fileApi.files(2)).resolves.toEqual([])
  })

  it('error 为空串 ⇒ 仍算读取成功（后端只在失败时填）', async () => {
    getMock.mockResolvedValue({ data: { entries: [{ name: 'ok', is_dir: true }], error: '' } })
    await expect(fileApi.files(3)).resolves.toEqual([{ name: 'ok', is_dir: true }])
  })
})

// 读不到目标目录时的措辞：不许把「无法确认」写成「没有同名项」，
// 也不许省掉「覆盖不可恢复」和「这次要动几项」。
describe('targetReadFailedNotice 说清不确定与后果', () => {
  it('移动/复制各自用对应动词，且都含「无法确认」「覆盖」「不可恢复」', () => {
    const mv = targetReadFailedNotice('move', 2)
    const cp = targetReadFailedNotice('copy', 2)
    expect(mv.body).toContain('移动')
    expect(cp.body).toContain('复制')
    for (const n of [mv, cp]) {
      expect(n.body).toContain('无法确认')
      expect(n.body).toContain('覆盖')
      expect(n.body).toContain('不可恢复')
      expect(n.title).toContain('无法确认')
    }
    expect(mv.body).not.toContain('复制')
  })

  it('把待处理项数写进文案（不是「若干项」）', () => {
    expect(targetReadFailedNotice('copy', 7).body).toContain('7 项')
    expect(targetReadFailedNotice('copy', 0).body).toContain('0 项')
  })
})

// 注：这里原本是一条「读真源码断言 doMoveCopy 真的弹了确认」的结构护栏，但本仓 tsconfig 不暴露
// node 类型（node:fs / process 一律 TS2307/2591，vue-tsc 整棵树红），静态护栏在此层走不通。
// 接线改由行为级用例证明：见 ../composables/useFileOps.movecopy_guard.test.ts（假 fm + mock
// ElMessageBox.confirm，断言预检失败时 confirm 被调、取消就一个字节都不动）。
