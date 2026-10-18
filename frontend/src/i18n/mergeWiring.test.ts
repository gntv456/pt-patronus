import { describe, expect, it } from 'vitest'
import { i18n, loadLocale } from './index'
import cameraBridge from './locales/cameraBridge'

// 端到端验证「指引真的进了 messages」——这正是之前 bridge.default[l] 取 undefined
// 时静默失效的地方：类型报错只出现在 typecheck，运行时无人察觉。
describe('i18n 合并指引模块', () => {
  it('loadLocale 后 cambridge.* 可被 t() 取到中文文案', async () => {
    await loadLocale('zh-CN')
    const t = i18n.global.t as (k: string) => string
    expect(t('cambridge.rtspTitle')).toBe(
      (cameraBridge.zh as unknown as { cambridge: Record<string, string> }).cambridge.rtspTitle,
    )
    // 不能是 key 原文（漏合并的典型症状）
    expect(t('cambridge.rtspTitle')).not.toBe('cambridge.rtspTitle')
    expect(t('cambridge.xiaomiStep3')).toContain('8554')
  })

  it('主表 key 未受影响（合并是叠加不是替换）', async () => {
    await loadLocale('zh-CN')
    const t = i18n.global.t as (k: string) => string
    expect(t('common.name')).toBeTruthy()
    expect(t('rp.noRtsp')).toBeTruthy()
  })
})
