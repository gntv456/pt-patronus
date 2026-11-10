// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const state = vi.hoisted(() => ({
  assessFail: false,
  contractFail: false,
  flFail: false,
  bfFail: false,
  saveptFail: false,
}))

vi.mock('@/api/assessment', () => ({
  assessmentApi: {
    all: vi.fn(async () => {
      if (state.assessFail) throw new Error('assessment list 500')
      return [
        { site_id: 1, status: 'SAFE', progress_percent: 50, remaining_days: 3, metrics: [] },
      ]
    }),
  },
}))
vi.mock('@/api/contract', () => ({
  contractApi: {
    list: vi.fn(async () => {
      if (state.contractFail) throw new Error('contract list 500')
      return [{ id: 9, site_id: 1, min_ratio: 1, min_seeding: 2, min_upload_gb: 0 }]
    }),
    check: vi.fn(async () => ({ status: 'SAFE', detail: 'ok' })),
  },
}))
vi.mock('@/api/freeleech', () => ({
  freeleechApi: {
    all: vi.fn(async () => {
      if (state.flFail) throw new Error('freeleech list 500')
      return [{ site_id: 1, notices: [{ title: 't', status: 'on', content: 'c' }] }]
    }),
  },
}))
vi.mock('@/api/brushflow', () => ({
  brushflowApi: {
    list: vi.fn(async () => {
      if (state.bfFail) throw new Error('brushflow list 500')
      return [{ id: 5, site_id: 1, name: 'bf', enabled: true, total_added: 0, total_deleted: 0, max_seeds: 0 }]
    }),
  },
}))
vi.mock('@/api/savept', () => ({
  saveptApi: {
    get: vi.fn(async () => {
      if (state.saveptFail) throw new Error('savept 500')
      return { cards: [{ local_site_id: 1, name: 's', status: 'SAFE' }] }
    }),
  },
}))
vi.mock('@/api/reseed', () => ({
  reseedApi: { tasks: vi.fn(async () => ({ tasks: [], total: 0, truncated: false })) },
}))

import { useSiteDetailTabs } from './useSiteDetailTabs'

// 读失败 ≠ 没有数据：列表接口挂了时各 tab 必须置失败位，
// 由 SiteDetail 渲染成「加载失败」而不是「无考核/无任务」。
describe('useSiteDetailTabs · 读取失败口径', () => {
  beforeEach(() => {
    state.assessFail = false
    state.contractFail = false
    state.flFail = false
    state.bfFail = false
    state.saveptFail = false
  })
  afterEach(() => {
    vi.clearAllMocks()
  })

  it('考核与契约列表都失败时置 assessFailed/contractFailed', async () => {
    state.assessFail = true
    state.contractFail = true
    const tabs = useSiteDetailTabs(
      () => 1,
      () => 'Site',
    )
    await tabs.loadTab('assessment')
    expect(tabs.assessFailed.value).toBe(true)
    expect(tabs.contractFailed.value).toBe(true)
    expect(tabs.assess.value).toBeNull()
    expect(tabs.contract.value).toBeNull()
  })

  it('单源失败只标记该源，另一源照常出数据', async () => {
    state.assessFail = true
    const tabs = useSiteDetailTabs(
      () => 1,
      () => 'Site',
    )
    await tabs.loadTab('assessment')
    expect(tabs.assessFailed.value).toBe(true)
    expect(tabs.contractFailed.value).toBe(false)
    expect(tabs.contract.value?.id).toBe(9)
  })

  it('正控：读取成功时失败位全为 false 且出数据', async () => {
    const tabs = useSiteDetailTabs(
      () => 1,
      () => 'Site',
    )
    await tabs.loadTab('assessment')
    expect(tabs.assessFailed.value).toBe(false)
    expect(tabs.contractFailed.value).toBe(false)
    expect(tabs.assess.value?.status).toBe('SAFE')
  })

  it('freeleech/brushflow/savept 失败各自置位并保持空值', async () => {
    state.flFail = true
    state.bfFail = true
    state.saveptFail = true
    const tabs = useSiteDetailTabs(
      () => 1,
      () => 'Site',
    )
    await tabs.loadTab('freeleech')
    await tabs.loadTab('brushflow')
    await tabs.loadTab('savept')
    expect(tabs.flFailed.value).toBe(true)
    expect(tabs.bfFailed.value).toBe(true)
    expect(tabs.saveptFailed.value).toBe(true)
    expect(tabs.fl.value).toBeNull()
    expect(tabs.bfTasks.value).toEqual([])
    expect(tabs.saveptCard.value).toBeNull()
  })

  it('clearTabState 清掉失败位', async () => {
    state.assessFail = true
    const tabs = useSiteDetailTabs(
      () => 1,
      () => 'Site',
    )
    await tabs.loadTab('assessment')
    expect(tabs.assessFailed.value).toBe(true)
    tabs.clearTabState()
    expect(tabs.assessFailed.value).toBe(false)
  })
})
