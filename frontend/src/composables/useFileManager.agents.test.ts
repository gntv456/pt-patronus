// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

// #288：Agent 列表读失败从前只写 `agents.value = []`，于是「没读到」和「这台主机真没配 Agent」
// 在存储对话框里是同一个空下拉——用户以为自己没建过，甚至去重复建一个。
// 这条把原因留在状态里（agentsError），界面拿它区分两种空态。

const state = vi.hoisted(() => ({
  agentsFail: null as null | { message: string },
  agents: [] as { id: number; name: string; online: boolean }[],
}))

vi.mock('@/api/monitor', () => ({
  agentApi: {
    list: vi.fn(async () => {
      if (state.agentsFail) throw { response: { status: 500 }, message: state.agentsFail.message }
      return state.agents
    }),
  },
}))
vi.mock('@/api/filemanager', () => ({
  fileApi: {
    list: vi.fn(async () => []),
    test: vi.fn(async () => ({ ok: true })),
    files: vi.fn(async () => []),
  },
}))
vi.mock('@/api/cloud', () => ({
  cloudApi: {
    listDrives: vi.fn(async () => []),
    test: vi.fn(async () => ({ ok: true })),
    browse: vi.fn(async () => []),
  },
}))
vi.mock('element-plus', () => ({
  ElMessage: { error: vi.fn(), warning: vi.fn(), success: vi.fn(), info: vi.fn() },
  ElMessageBox: { confirm: vi.fn() },
}))

import { useFileManager } from './useFileManager'

beforeEach(() => {
  setActivePinia(createPinia())
  state.agentsFail = null
  state.agents = []
})

describe('useFileManager · Agent 列表读取口径', () => {
  it('读失败时把后端那句原因留在状态里（不再只留一个空数组）', async () => {
    state.agentsFail = { message: '监控模块未启用：请先在设置里开启后重试' }
    const fm = useFileManager()
    await fm.loadAgents()
    expect(fm.agents.value).toEqual([])
    expect(fm.agentsError.value).toContain('监控模块未启用')
  })

  it('正控：读成功（哪怕真的是空列表）不许留下失败痕迹', async () => {
    state.agents = []
    const fm = useFileManager()
    await fm.loadAgents()
    expect(fm.agentsError.value).toBe('')

    // 先失败再成功：原因必须被清掉，否则一次抖动会长期挂在面板上
    state.agentsFail = { message: 'boom' }
    await fm.loadAgents()
    expect(fm.agentsError.value).toContain('boom')
    state.agentsFail = null
    state.agents = [{ id: 1, name: 'nas-01', online: true }]
    await fm.loadAgents()
    expect(fm.agentsError.value).toBe('')
    expect(fm.agents.value).toHaveLength(1)
  })

  it('拿不到 message 的异常也要出声，而不是静默成空下拉', async () => {
    state.agentsFail = { message: '' }
    const fm = useFileManager()
    await fm.loadAgents()
    expect(fm.agentsError.value).toBeTruthy()
  })
})
