// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// @vitest-environment jsdom
// #258：一键签到的「站点清单读不出来」与「没有启用的站点」是两件事。
// 后端修好后信封里有 list_error，但如果这个 composable 只读 results，
// 界面照样会说「没有启用的站点」——所以判据落在按钮实际打到的这个函数上。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'

const state = vi.hoisted(() => ({
  results: [] as Array<Record<string, unknown>>,
  listError: '',
}))

const msg = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
  warning: vi.fn(),
  info: vi.fn(),
}))

vi.mock('@/api/site', () => ({
  siteApi: {
    checkinAll: vi.fn(async () => ({ results: state.results, listError: state.listError })),
  },
}))

vi.mock('element-plus', () => ({
  ElMessage: msg,
  ElMessageBox: { confirm: vi.fn(async () => true) },
}))

const { useSitesActions } = await import('@/composables/useSitesActions')

function actions() {
  return useSitesActions(ref([]) as never, ref([]) as never, vi.fn() as never, vi.fn() as never)
}

describe('一键签到把「清单读不出」说成故障而不是「没站」', () => {
  beforeEach(() => {
    state.results = []
    state.listError = ''
    for (const m of [msg.success, msg.error, msg.warning, msg.info]) m.mockReset()
  })

  it('list_error 有值 ⇒ error 通道 + 带上后端原文，且不说「没有启用的站点」', async () => {
    state.listError = '读取启用站点清单失败: no such table: sites'
    await actions().checkinAll()
    expect(msg.error).toHaveBeenCalledTimes(1)
    const text = String(msg.error.mock.calls[0][0])
    expect(text).toContain('站点清单读不出来')
    expect(text).toContain('no such table: sites')
    expect(text).not.toContain('没有启用的站点')
    expect(msg.info).not.toHaveBeenCalled()
    expect(msg.success).not.toHaveBeenCalled()
  })

  it('正控：清单空但读得出来 ⇒ 仍说「没有启用的站点」，不许报故障', async () => {
    await actions().checkinAll()
    expect(msg.error).not.toHaveBeenCalled()
    const text = String(msg.info.mock.calls[0][0])
    expect(text).toContain('没有启用的站点')
  })

  it('正控：有站有结果 ⇒ 走原来的成功/失败汇总，不被新分支吃掉', async () => {
    state.results = [
      { site_id: 1, site_name: 'A', success: true, message: '签到成功' },
      { site_id: 2, site_name: 'B', success: false, error: '登录态已失效' },
    ]
    await actions().checkinAll()
    expect(msg.error).not.toHaveBeenCalled()
    expect(msg.info).not.toHaveBeenCalled()
    const text = String(msg.warning.mock.calls[0][0])
    expect(text).toContain('成功 1')
    expect(text).toContain('失败 1')
    expect(text).toContain('B')
  })
})
