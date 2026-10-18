// @vitest-environment jsdom
// #317 告警面板五个动作（标记已读 / 确认 / 静默 / 删除 / 清空）此前一个 try/catch 都没有。
// 后端这些出口有真会发生的 400/404（#301/#310 那批刚把中文原因接进 invalidInput 载体：
// 告警已被别人清掉 → monitor_alert_not_found；静默时间越界/在过去 → 参数有误）。
// 异常逃出 async 的 click handler ⇒ Promise rejection 没人接：弹窗关了、列表没变、也不吭声，
// 用户读到的就是「点了没反应」（同 #206 收件箱、#268 移动端删除会话那族）。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'

import { monitorApi, type MonitorAlert } from '@/api/monitor'
import { useDeviceCrud } from './useDeviceCrud'

// 替身只喂动作函数真正读到的那几个字段（id / level / metric / message），其余走 unknown 转接。
const ALERT = {
  id: 7,
  level: 'danger',
  metric: 'disk_usage',
  message: '磁盘 92%',
  unread: true,
} as unknown as MonitorAlert

function makeCrud(reload = vi.fn(async () => {})) {
  const crud = useDeviceCrud(
    { isAdmin: true },
    {
      activeWorkspace: ref('monitor') as never,
      activeTab: ref('host') as never,
      credentials: ref([]) as never,
      reload,
      setWorkspace: vi.fn(),
    },
  )
  return { crud, reload }
}

beforeEach(() => {
  vi.restoreAllMocks()
})

describe('告警动作失败必须出声', () => {
  const cases: {
    name: string
    api: keyof typeof monitorApi
    run: (c: ReturnType<typeof makeCrud>['crud']) => Promise<unknown>
  }[] = [
    { name: '标记已读', api: 'readAlert', run: (c) => c.onReadAlert(ALERT) },
    { name: '确认告警', api: 'ackAlert', run: (c) => c.onAckAlert(ALERT) },
    { name: '静默告警', api: 'silenceAlert', run: (c) => c.onSilenceAlert(ALERT) },
    { name: '删除告警', api: 'deleteAlert', run: (c) => c.onDeleteAlert(ALERT) },
    { name: '清空告警', api: 'clearAlerts', run: (c) => c.onClearAlerts() },
  ]

  for (const c of cases) {
    it(`${c.name}：后端回了中文原因就要说出来，且不许把异常抛回 click handler`, async () => {
      // 删除/清空先过一道确认框，这里当成用户点了「确定」
      vi.spyOn(ElMessageBox, 'confirm').mockResolvedValue('confirm' as never)
      const errSpy = vi.spyOn(ElMessage, 'error').mockImplementation(() => ({}) as never)
      const okSpy = vi.spyOn(ElMessage, 'success').mockImplementation(() => ({}) as never)
      // 替换 api 对象上的方法：这些方法签名各不相同，替身只管 reject
      const target = monitorApi as unknown as Record<string, (...a: unknown[]) => unknown>
      target[c.api] = vi.fn().mockRejectedValue(new Error('该告警已被清掉，请刷新列表'))

      await expect(c.run(makeCrud().crud)).resolves.toBeUndefined()

      expect(errSpy, '失败要弹后端那句中文原因').toHaveBeenCalled()
      expect(String(errSpy.mock.calls[0][0])).toContain('该告警已被清掉')
      expect(okSpy, '失败时不许顺手弹成功').not.toHaveBeenCalled()
    })
  }
})

describe('成功路径不许被顺手改坏', () => {
  it('确认/静默/删除/清空 仍各自弹原来的成功提示并重读列表', async () => {
    vi.spyOn(ElMessageBox, 'confirm').mockResolvedValue('confirm' as never)
    const okSpy = vi.spyOn(ElMessage, 'success').mockImplementation(() => ({}) as never)
    const errSpy = vi.spyOn(ElMessage, 'error').mockImplementation(() => ({}) as never)
    monitorApi.ackAlert = vi.fn(async () => ({})) as never
    monitorApi.silenceAlert = vi.fn(async () => ({})) as never
    monitorApi.deleteAlert = vi.fn(async () => ({ deleted: 7 })) as never
    monitorApi.clearAlerts = vi.fn(async () => ({ cleared: 3 })) as never

    const { crud, reload } = makeCrud()
    await crud.onAckAlert(ALERT)
    expect(okSpy.mock.calls.at(-1)?.[0]).toContain('已确认')
    await crud.onSilenceAlert(ALERT)
    expect(okSpy.mock.calls.at(-1)?.[0]).toContain('已静默')
    await crud.onDeleteAlert(ALERT)
    expect(okSpy.mock.calls.at(-1)?.[0]).toContain('已删除')
    await crud.onClearAlerts()
    expect(okSpy.mock.calls.at(-1)?.[0]).toContain('已清空')
    expect(errSpy).not.toHaveBeenCalled()
    expect(reload, '四个动作都要重读列表').toHaveBeenCalledTimes(4)
  })

  it('标记已读成功保持既有形状：不弹成功 toast，但要重读列表', async () => {
    const okSpy = vi.spyOn(ElMessage, 'success').mockImplementation(() => ({}) as never)
    monitorApi.readAlert = vi.fn(async () => ({ read: 7 })) as never
    const { crud, reload } = makeCrud()
    await crud.onReadAlert(ALERT)
    expect(okSpy).not.toHaveBeenCalled()
    expect(reload).toHaveBeenCalled()
  })
})
