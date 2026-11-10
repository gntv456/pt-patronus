// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { beforeEach, describe, expect, it, vi } from 'vitest'

const state = vi.hoisted(() => ({
  cfgFail: false,
  dlsFail: false,
  drivesFail: false,
}))

vi.mock('vue-router', () => ({ useRoute: () => ({ query: {} }), useRouter: () => ({ push: vi.fn() }) }))
vi.mock('element-plus', () => ({
  ElMessage: { error: vi.fn(), warning: vi.fn(), success: vi.fn(), info: vi.fn() },
  ElMessageBox: { confirm: vi.fn() },
}))
vi.mock('@/api/transfer', () => ({
  transferApi: {
    getConfig: vi.fn(async () => {
      if (state.cfgFail) throw new Error('config 500')
      return { path_mappings: [], failed_retry_minutes: 360 }
    }),
    setConfig: vi.fn(async () => ({})),
    list: vi.fn(async () => []),
    summary: vi.fn(async () => ({})),
    setCategoriesConfig: vi.fn(async () => ({})),
  },
}))
vi.mock('@/api/downloader', () => ({
  downloaderApi: {
    list: vi.fn(async () => {
      if (state.dlsFail) throw new Error('dls 500')
      return [{ id: 1, name: 'qb' }]
    }),
    torrents: vi.fn(async () => []),
  },
}))
vi.mock('@/api/cloud', () => ({
  cloudApi: {
    listDrives: vi.fn(async () => {
      if (state.drivesFail) throw new Error('drives 500')
      return [{ id: 2, name: '115', type: '115' }]
    }),
  },
}))

import { useTransfer } from './useTransfer'

// 配置抽屉的三个数据源（配置本体/下载器/云盘）读失败 ≠ 没配置：
// cfgLoadFailed 必须点名失败源，抽屉据此显示「加载失败」而不是空下拉。
describe('useTransfer · 配置抽屉数据源读失败口径', () => {
  beforeEach(() => {
    state.cfgFail = false
    state.dlsFail = false
    state.drivesFail = false
  })

  it('下载器与云盘列表都失败时点名两者，配置本体照常回填', async () => {
    state.dlsFail = true
    state.drivesFail = true
    const t = useTransfer()
    await t.openConfig()
    expect(t.cfgLoadFailed.value).toEqual(['downloader', 'cloud'])
    expect(t.cfg.value.failed_retry_minutes).toBe(360)
    expect(t.showConfig.value).toBe(true)
  })

  it('配置本体失败时点名 config，下拉列表照常加载', async () => {
    state.cfgFail = true
    const t = useTransfer()
    await t.openConfig()
    expect(t.cfgLoadFailed.value).toEqual(['config'])
    expect(t.cloudDrives.value).toHaveLength(1)
  })

  it('正控：三源都成功时无失败点名单', async () => {
    const t = useTransfer()
    await t.openConfig()
    expect(t.cfgLoadFailed.value).toEqual([])
    expect(t.cloudDrives.value).toHaveLength(1)
  })
})
