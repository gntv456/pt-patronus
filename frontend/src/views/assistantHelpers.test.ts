import { describe, expect, it } from 'vitest'
import { allowAllToggleBlocked, cfgReadFailText, toolsReadFailText } from './assistantHelpers'

// #267：AI 助手设置面板里「工具目录 / 配置本体读不出来」绝不能被当成「没有」。
// 后端把空 enabled_tools 解释成全开（aiagent/tools.go: toolAllowed 空表 ⇒ true），
// 所以一次读取失败如果放行「启用全部 ↔ 自选」这一改，用户点到的其实是「全部允许」。
// 面板要真挂载才碰得到（jsdom 点不进 el-dialog），先把判定与文案收在这三个纯函数里断言。
describe('assistantHelpers 配置面板的读取失败判定', () => {
  it('目录读不出来 ⇒ 「启用全部工具」开关必须锁死', () => {
    expect(allowAllToggleBlocked('403 Forbidden')).toBe(true)
  })

  it('正控：目录读得好时开关可用、提示原样不动', () => {
    expect(allowAllToggleBlocked('')).toBe(false)
    expect(toolsReadFailText('', '关闭后可按风险勾选')).toBe('关闭后可按风险勾选')
  })

  it('读失败要说清是目录读不出来、带上原因、并点明「空清单=全开」这个后果', () => {
    const txt = toolsReadFailText('network down', '正常提示')
    expect(txt).toContain('工具目录读不出来')
    expect(txt).toContain('network down')
    expect(txt).toContain('全部允许')
  })

  it('配置本体读不出来时的文案要禁止打开面板，且原因不能是空串', () => {
    expect(cfgReadFailText('HTTP 500')).toContain('HTTP 500')
    expect(cfgReadFailText('')).toContain('未知原因')
    expect(cfgReadFailText('')).toContain('未打开')
  })
})
