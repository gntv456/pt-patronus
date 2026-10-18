import { beforeEach, describe, expect, it, vi } from 'vitest'

// 这一条补的是 #244 的接线证明：`targetReadFailedNotice` 自己绿不代表 doMoveCopy 真的在用它。
// 后端 move/copy 是「同名静默覆盖」（不可逆），所以「读不到目标目录」绝不能被当成「预检通过」。
// 这里不起组件、不碰网络：假 fm 只提供 doMoveCopy 读到的那几个 value，
// element-plus 与 fileApi 的网络面整块 mock，targetReadFailedNotice 走真实实现（用 importOriginal 保住）。
const { filesMock, moveMock, copyMock, confirmMock, msg } = vi.hoisted(() => ({
  filesMock: vi.fn(),
  moveMock: vi.fn(),
  copyMock: vi.fn(),
  confirmMock: vi.fn(),
  msg: { error: vi.fn(), success: vi.fn(), warning: vi.fn() },
}))

vi.mock('@/api/filemanager', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api/filemanager')>()
  return {
    ...actual,
    fileApi: { ...actual.fileApi, files: filesMock, move: moveMock, copy: copyMock },
  }
})
vi.mock('@/api/cloud', () => ({ cloudApi: new Proxy({}, { get: () => vi.fn() }) }))
vi.mock('element-plus', () => ({ ElMessage: msg, ElMessageBox: { confirm: confirmMock } }))

import { useFileOps } from './useFileOps'

function fakeFm(mode: 'move' | 'copy') {
  return {
    curFs: { value: { id: 7 } },
    moveCopySources: { value: [{ path: '/in/x.mkv', name: 'x.mkv' }] },
    moveCopyPath: { value: '/out/' },
    moveCopyMode: { value: mode },
    moveCopyVisible: { value: true },
    submitting: { value: false },
    refresh: vi.fn().mockResolvedValue(undefined),
  } as never
}

describe('doMoveCopy：目标目录读不到时不当作「没有同名项」', () => {
  beforeEach(() => {
    for (const m of [filesMock, moveMock, copyMock, confirmMock, msg.error, msg.success]) m.mockReset()
  })

  it('预检失败 ⇒ 弹「无法确认」确认，取消就一个字节都不动', async () => {
    filesMock.mockRejectedValue(new Error('WebDAV 目录列举失败：403 Forbidden'))
    confirmMock.mockRejectedValue('cancel') // 用户取消 = ElMessageBox 的 reject 形状
    const fm = fakeFm('move')
    await useFileOps(fm).doMoveCopy()

    expect(confirmMock).toHaveBeenCalledTimes(1)
    const [body, title] = confirmMock.mock.calls[0]
    expect(String(body)).toContain('无法确认')
    expect(String(body)).toContain('不可恢复')
    expect(String(title)).toContain('无法确认')
    expect(moveMock).not.toHaveBeenCalled()
    expect(copyMock).not.toHaveBeenCalled()
  })

  it('预检失败但用户明确同意 ⇒ 才继续执行（闸门不是禁令）', async () => {
    filesMock.mockRejectedValue(new Error('权限不足'))
    confirmMock.mockResolvedValue('confirm')
    moveMock.mockResolvedValue({ ok: true })
    const fm = fakeFm('move')
    await useFileOps(fm).doMoveCopy()

    expect(confirmMock).toHaveBeenCalledTimes(1)
    expect(moveMock).toHaveBeenCalledTimes(1)
    expect(moveMock).toHaveBeenCalledWith(7, '/in/x.mkv', '/out/')
    expect(msg.success).toHaveBeenCalledWith('已移动 1 项')
  })

  it('目标真有同名 ⇒ 弹的是「目标存在同名 1 项」，取消同样不动', async () => {
    filesMock.mockResolvedValue([{ name: 'x.mkv', is_dir: false }])
    confirmMock.mockRejectedValue('cancel')
    const fm = fakeFm('copy')
    await useFileOps(fm).doMoveCopy()

    expect(confirmMock).toHaveBeenCalledTimes(1)
    expect(String(confirmMock.mock.calls[0][0])).toContain('目标存在同名 1 项')
    expect(copyMock).not.toHaveBeenCalled()
  })

  it('目标读得到且无同名 ⇒ 不打扰用户，直接执行（正控：新闸门没有把所有操作都拦停）', async () => {
    filesMock.mockResolvedValue([{ name: 'other.mkv', is_dir: false }])
    copyMock.mockResolvedValue({ ok: true })
    const fm = fakeFm('copy')
    await useFileOps(fm).doMoveCopy()

    expect(confirmMock).not.toHaveBeenCalled()
    expect(copyMock).toHaveBeenCalledWith(7, '/in/x.mkv', '/out/')
    expect(msg.success).toHaveBeenCalledWith('已复制 1 项')
  })
})
