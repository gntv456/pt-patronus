// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'

// #252：探活「没测出来」与「测出来是离线」是两件事。后端 monitor.go:99 在前一种回
// {ok:false, online:false, error:'原因'}，从前界面只看 online ⇒ 一律说成「设备离线」，
// 用户会去查那台机器，而真正坏的是凭据/链路。这里用文案的措辞本身做 A/B。
const { probeMock, msg, sshFix } = vi.hoisted(() => ({
  probeMock: vi.fn(),
  msg: { warning: vi.fn(), error: vi.fn(), success: vi.fn() },
  sshFix: vi.fn(),
}))

vi.mock('@/api/monitor', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api/monitor')>()
  return { ...actual, monitorApi: { ...actual.monitorApi, probe: probeMock } }
})
vi.mock('element-plus', () => ({ ElMessage: msg, ElMessageBox: { confirm: vi.fn() } }))
// 只替身掉「弹修复面板」这个副作用，判据 isSSHAuthError 走真实实现。
vi.mock('@/utils/ssh-fix', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/utils/ssh-fix')>()
  return { ...actual, showSSHAuthFix: sshFix }
})

import { useDeviceCrud } from './useDeviceCrud'

const target = { id: 7, name: '绿联 NAS', type: 'nas' } as never

function crud() {
  const reload = vi.fn().mockResolvedValue(undefined)
  return useDeviceCrud({ isAdmin: true } as never, {
    activeWorkspace: ref('monitor') as never,
    activeTab: ref('nas') as never,
    credentials: ref([]) as never,
    reload,
    setWorkspace: vi.fn(),
  })
}

describe('设备探活把「没测出来」与「测出离线」分开说', () => {
  beforeEach(() => {
    probeMock.mockReset()
    sshFix.mockReset()
    for (const m of [msg.warning, msg.error, msg.success]) m.mockReset()
  })

  it('ok:false ⇒ 说「探活未完成」并带上后端原因，不许写成设备离线', async () => {
    probeMock.mockResolvedValue({ ok: false, online: false, error: 'SSH 凭据已被删除，请重新绑定' })
    await crud().onProbe(target)
    expect(msg.warning).toHaveBeenCalledTimes(1)
    const text = String(msg.warning.mock.calls[0][0])
    expect(text).toContain('探活未完成')
    expect(text).toContain('SSH 凭据已被删除')
    expect(text).not.toContain('离线')
  })

  it('ok:true 且 online:false ⇒ 仍然要说「离线」（正控：新措辞不许吃掉真离线）', async () => {
    probeMock.mockResolvedValue({ ok: true, online: false })
    await crud().onProbe(target)
    const text = String(msg.warning.mock.calls[0][0])
    expect(text).toContain('离线')
    expect(text).not.toContain('探活未完成')
  })

  it('在线 ⇒ 一句 success，别的都不弹', async () => {
    probeMock.mockResolvedValue({ ok: true, online: true })
    await crud().onProbe(target)
    expect(msg.success).toHaveBeenCalledWith('绿联 NAS 在线')
    expect(msg.warning).not.toHaveBeenCalled()
  })

  it('SSH 认证类原因 ⇒ 走「去修凭据」面板而不是只报一句文案', async () => {
    probeMock.mockResolvedValue({
      ok: false,
      online: false,
      error: 'ssh: unable to authenticate, attempted methods [none publickey]',
    })
    await crud().onProbe(target)
    expect(sshFix).toHaveBeenCalledTimes(1)
    expect(msg.warning).not.toHaveBeenCalled()
  })

  it('请求本身失败 ⇒ 把拦截器改写过的原因带上，不压成「探活失败」', async () => {
    probeMock.mockRejectedValue(new Error('网络连接失败，请检查网络'))
    await crud().onProbe(target)
    expect(msg.error).toHaveBeenCalledTimes(1)
    expect(String(msg.error.mock.calls[0][0])).toContain('网络连接失败')
  })
})
