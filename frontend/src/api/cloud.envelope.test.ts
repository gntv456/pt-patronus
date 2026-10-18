// @vitest-environment jsdom
// #246 的链路收口：后端把「云盘账号已被删除」这类原因写成 404 信封里的中文
// （errcode Respond ⇒ {code, message, error}，message=给人看的那句）。
// 界面 toast 走的是 `e?.message || '删除失败'`，而 e.message 由 http.ts 的错误拦截器改写——
// 这里用**真 http 实例 + 真拦截器**（只把传输层换成假 adapter）验这句中文会不会在半路被兜底文案换掉。
import { afterEach, describe, expect, it } from 'vitest'
import { http } from './http'
import { cloudApi } from './cloud'

const realAdapter = http.defaults.adapter

// 用后端真实信封喂假传输层（status + data 形状与 errcode.Respond 一致）。
function failWith(status: number, data: Record<string, unknown>): void {
  http.defaults.adapter = async (config) => {
    const err = new Error(`Request failed with status code ${status}`) as Error & Record<string, unknown>
    err.isAxiosError = true
    err.config = config
    err.response = { status, data, config, headers: {} }
    throw err
  }
}

afterEach(() => {
  http.defaults.adapter = realAdapter
})

describe('云盘 404 的中文原因要活着走到调用方', () => {
  it('云盘账号不存在 ⇒ 拿到后端那句「云盘账号不存在…」，不是通用兜底', async () => {
    failWith(404, {
      code: 'cloud_drive_not_found',
      message: '云盘账号不存在，可能已被删除，请重新选择云盘',
      error: '云盘账号不存在，可能已被删除，请重新选择云盘',
    })
    const e = await cloudApi.removeDrive(999).catch((err: Error) => err)
    expect(e).toBeInstanceOf(Error)
    expect((e as Error).message).toContain('云盘账号不存在')
    expect((e as Error).message).not.toContain('请求的资源不存在')
    // 机器码要留在 err.code 上（业务分支靠它，不能被文案化吃掉）。
    expect((e as Error & { code?: string }).code).toBe('cloud_drive_not_found')
  })

  it('STRM 规则 404 ⇒ 说清是规则或其云盘账号没了', async () => {
    failWith(404, {
      code: 'strm_rule_not_found',
      message: 'STRM 规则或其云盘账号不存在，可能已被删除，请刷新后重试',
      error: 'STRM 规则或其云盘账号不存在，可能已被删除，请刷新后重试',
    })
    const e = await cloudApi.removeRule(7).catch((err: Error) => err)
    expect((e as Error).message).toContain('STRM 规则或其云盘账号不存在')
  })

  // 对照：这条证明上面不是因为「任何 404 都显示后端那句」而假绿——
  // 信封里没有文案时才该落 i18n 兜底。
  it('正控：404 且信封没带文案 ⇒ 才用兜底「请求的资源不存在」', async () => {
    failWith(404, {})
    const e = await cloudApi.listDrives().catch((err: Error) => err)
    expect((e as Error).message).toContain('请求的资源不存在')
  })

  it('200 + 空 drives 仍按数组返回（读成功不该被当成失败）', async () => {
    http.defaults.adapter = async (config) => ({
      status: 200,
      statusText: 'OK',
      data: { drives: [] },
      headers: {},
      config,
    })
    await expect(cloudApi.listDrives()).resolves.toEqual([])
  })
})
