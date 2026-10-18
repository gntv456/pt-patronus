import { describe, it, expect } from 'vitest'

// 回归钉（1.03.22 白屏 P0）：useNoticeMeta.ts 曾在模块顶层调用 useI18n()，
// Vite 把它拆为独立 chunk 后，动态 import() 在无 setup 上下文求值即抛
// I18nError(26)=MUST_BE_CALL_SETUP_TOP → vue-router 导航中止 → 全站白屏。
// 顶层若有任何 setup 专属 API 调用，本测试的动态导入会直接拒绝。
describe('useNoticeMeta 模块可导入性', () => {
  it('无 setup 上下文导入不抛错（顶层不得调用 useI18n 等 setup 专属 API）', async () => {
    const mod = await import('@/composables/useNoticeMeta')
    expect(typeof mod.useNoticeMeta).toBe('function')
    expect(mod.noticeSourceRoute.new_mail).toBe('messages')
  })
})
