// @vitest-environment jsdom
// #246 的第二层：404 那句中文到了调用方之后，界面到底弹不弹得出来。
// 这里的错误对象是上一份用例（cloud.envelope.test.ts）证明的「拦截器改写后的 err」形状：
// message=后端原话。判据是 toast 用原话，而不是退回复里的「删除失败 / 加载规则失败」。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'

const msg = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
  warning: vi.fn(),
  info: vi.fn(),
}))

const api = vi.hoisted(() => ({
  removeRule: vi.fn(),
  listRules: vi.fn(),
  listRecords: vi.fn(),
  generate: vi.fn(),
  generateAll: vi.fn(),
  syncSubscriptions: vi.fn(),
  createRule: vi.fn(),
  updateRule: vi.fn(),
  removeRecord: vi.fn(),
}))

vi.mock('@/api/cloud', () => ({ cloudApi: api }))
vi.mock('element-plus', () => ({
  ElMessage: msg,
  ElMessageBox: { confirm: vi.fn(async () => true) },
}))

const { useCloudStrmRules } = await import('@/composables/useCloudStrmRules')

const DRIVE = { id: 3, name: '115', type: '115' } as never

function rules() {
  return useCloudStrmRules(ref([DRIVE]) as never, vi.fn() as never)
}

const NOT_FOUND = new Error('STRM 规则或其云盘账号不存在，可能已被删除，请刷新后重试')
;(NOT_FOUND as Error & { code: string }).code = 'strm_rule_not_found'

describe('STRM 规则操作把 404 原话弹出来', () => {
  beforeEach(() => {
    for (const m of [msg.success, msg.error, msg.warning, msg.info]) m.mockReset()
    api.removeRule.mockReset()
    api.listRules.mockReset()
  })

  it('删除失败 ⇒ toast 里是后端那句「…不存在，可能已被删除」，不是「删除失败」', async () => {
    api.removeRule.mockRejectedValue(NOT_FOUND)
    await rules().removeRule({ id: 7, name: '我的规则' } as never)
    expect(msg.error).toHaveBeenCalledTimes(1)
    const text = String(msg.error.mock.calls[0][0])
    expect(text).toContain('STRM 规则或其云盘账号不存在')
    expect(text).not.toBe('删除失败')
  })

  it('加载规则失败 ⇒ 同样带上原文（用户才知道是账号被删了而不是列表接口坏了）', async () => {
    api.listRules.mockRejectedValue(new Error('云盘账号不存在，可能已被删除，请重新选择云盘'))
    await rules().loadRules()
    const text = String(msg.error.mock.calls[0][0])
    expect(text).toContain('云盘账号不存在')
    expect(text).not.toBe('加载规则失败')
  })

  it('正控：删除成功 ⇒ 走「已删除」，不许把成功也报成故障', async () => {
    api.removeRule.mockResolvedValue({ ok: true })
    api.listRules.mockResolvedValue([])
    await rules().removeRule({ id: 7, name: 'ok' } as never)
    expect(msg.error).not.toHaveBeenCalled()
    expect(msg.success).toHaveBeenCalledWith('已删除')
  })
})
