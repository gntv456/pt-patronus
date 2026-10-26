import { describe, expect, it } from 'vitest'
import { translateErrcode } from './errcodeI18n'

// 后端写了「这一条请求专属的原因」时，英文界面不许用码的固定译文整句覆盖。
// （#69 的下游一半：后端补上了「缺少必填项 platform」，这里保证它到得了用户眼前。）
describe('translateErrcode 对 specific 原因不覆盖', () => {
  const reason = '缺少必填项 platform'

  it('specific=true 时原样显示后端原因（en-US）', () => {
    const got = translateErrcode('invalid_input', reason, 'en-US', true)
    expect(got).toBe(reason)
  })

  it('没带 specific 时仍按码查固定译文（en-US 老行为不变）', () => {
    const got = translateErrcode('invalid_input', '请求有参数后端无法接受', 'en-US')
    expect(got).not.toBe('请求有参数后端无法接受')
    expect(got.toLowerCase()).toContain('could not accept')
  })

  it('中文界面两种情况都回原文', () => {
    const gotZh = translateErrcode('invalid_input', reason, 'zh-CN', true)
    expect(gotZh).toBe(reason)
    expect(translateErrcode('invalid_input', reason, 'zh-CN')).toBe(reason)
  })

  it('认不出的 code 仍旧回原文（这条链不能倒退）', () => {
    expect(translateErrcode('brand_new_code', reason, 'en-US')).toBe(reason)
  })

  it('正控：不传 specific 参数时默认走查表（否则上面第一条恒真）', () => {
    expect(translateErrcode('invalid_input', reason, 'en-US')).not.toBe(reason)
  })
})
