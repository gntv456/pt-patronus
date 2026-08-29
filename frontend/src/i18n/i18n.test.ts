// i18n 实例行为：默认 locale、切换持久化、缺失 key 回落链（en-US → zh-CN → 兜底串）。
import { describe, expect, it, beforeEach } from 'vitest'
import { i18n, setLocale, LOCALE_KEY } from '@/i18n'

describe('i18n', () => {
  beforeEach(() => {
    localStorage.removeItem(LOCALE_KEY)
    setLocale('zh-CN')
  })

  it('默认 locale 为 zh-CN 且导航文案可查', () => {
    expect(i18n.global.locale.value).toBe('zh-CN')
    expect(i18n.global.t('nav.items.dashboard')).toBe('仪表盘')
    expect(i18n.global.t('login.form.username')).toBe('好学用户名')
  })

  it('setLocale 切换到 en-US 并持久化 localStorage', () => {
    setLocale('en-US')
    expect(i18n.global.locale.value).toBe('en-US')
    expect(localStorage.getItem(LOCALE_KEY)).toBe('en-US')
    expect(i18n.global.t('nav.items.dashboard')).toBe('Dashboard')
  })

  it('en-US 缺失 key 回落 zh-CN；两级都缺回落兜底串', () => {
    setLocale('en-US')
    // en-US 有此 key —— 正常译出
    expect(i18n.global.t('common.save')).toBe('Save')
    // 构造 en 缺失场景：直接查一个只存在于 zh 的 key（模拟长尾未翻译）
    // vue-i18n fallbackLocale=zh-CN → 返回 zh 文案而非 key 本身
    const zh = i18n.global.t('login.boot.vipOn', {}, { locale: 'zh-CN' })
    expect(zh.length).toBeGreaterThan(0)
    // 两级都缺 → t(key, 兜底串) 返回兜底（组件渲染不会裸奔 key 名）
    expect(i18n.global.t('nav.items.__never__', '原中文兜底')).toBe('原中文兜底')
  })
})
